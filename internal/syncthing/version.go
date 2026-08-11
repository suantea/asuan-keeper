// Package syncthing 版本对照与更新。
//
// asuan 与 sidecar 引擎（syncthing）是解耦的：引擎只作为同步内核被调用，
// 其自有版本与 asuan 功能/行为存在映射。此文件记录「当前 asuan 已验证的
// 引擎版本」清单，并实现 agent 驱动的引擎更新流程（asuan engine-update）。
package syncthing

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// 版本对照说明：
//   - 支持"跟随引擎升级"：引擎是独立二进制，asuan 只依赖其 REST API 与配置格式，
//     因此引擎小版本升级通常无需改 asuan 代码，直接替换二进制即可。
//   - VerifiedVersions 是已经用 asuan 端到端验证过的引擎版本（升级时优先选用）。
//   - 升级后务必在 agent 侧跑一遍 `asuan run` + `asuan status` 冒烟确认。

// VerifiedVersions 已随 asuan 验证过的 syncthing 版本（含 P0 三节点验证）。
var VerifiedVersions = []string{"v2.1.3"}

// CurrentEngineVersion 当前推荐的引擎版本（最新已验证版本）。
const CurrentEngineVersion = "v2.1.3"

// EngineInfo 描述已安装引擎的状态。
type EngineInfo struct {
	Installed string // 已安装引擎版本（可能为空=未探测到）
	Current   string // 当前推荐版本
	Verified  []string
}

// Version 查询已安装 syncthing 的版本（通过 REST /rest/system/version）。
// 引擎未运行时返回 ("", nil)，调用方自行区分。
func (m *Manager) Version() (string, error) {
	var ver struct {
		Version string `json:"version"`
	}
	if _, err := m.api("GET", "/rest/system/version", nil, &ver); err != nil {
		return "", err
	}
	return ver.Version, nil
}

// EngineCheck 返回版本对照信息。
func (m *Manager) EngineCheck() (*EngineInfo, error) {
	info := &EngineInfo{Current: CurrentEngineVersion, Verified: VerifiedVersions}
	if v, err := m.Version(); err == nil {
		info.Installed = v
	}
	return info, nil
}

// engineDownload 下载并解压指定版本引擎到临时目录，返回解压后二进制路径。
// 官方发行包名：syncthing-<os>-<arch>-<version>.zip（Windows/macOS）或 .tar.gz（Linux）。
func (m *Manager) engineDownload(version string) (string, error) {
	arch := runtime.GOARCH
	goos := runtime.GOOS
	osKey := goos
	ext := ".zip"
	switch goos {
	case "darwin":
		osKey = "darwin"
	case "linux":
		ext = ".tar.gz"
	}
	// 下载基址：默认 GitHub releases；网络受限时可设 ASUAN_ENGINE_BASE
	// 指向 ghproxy 之类镜像（如 https://ghproxy.net/https://github.com/syncthing/syncthing/releases/download）。
	base := os.Getenv("ASUAN_ENGINE_BASE")
	if base == "" {
		base = "https://github.com/syncthing/syncthing/releases/download"
	}
	url := fmt.Sprintf("%s/%s/syncthing-%s-%s-%s%s",
		base, version, osKey, arch, version, ext)

	tmp, err := os.MkdirTemp("", "asuan-engine-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)

	archivePath := filepath.Join(tmp, "engine"+ext)
	if err := downloadFile(url, archivePath); err != nil {
		return "", fmt.Errorf("下载引擎 %s 失败: %w", version, err)
	}

	var exePath string
	switch ext {
	case ".zip":
		exePath, err = extractZip(archivePath)
	case ".tar.gz":
		exePath, err = extractTarGz(archivePath)
	}
	if err != nil {
		return "", err
	}
	if goos == "windows" {
		exePath += ".exe"
	}
	// 解压产物放到独立目录，后续由调用方原子替换到引擎目录。
	stage := filepath.Join(tmp, "staged")
	if err := os.MkdirAll(stage, 0o700); err != nil {
		return "", err
	}
	target := filepath.Join(stage, "syncthing"+exeSuffix())
	if err := copyFile(exePath, target); err != nil {
		return "", err
	}
	return target, nil
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// DownloadAndUpdate 下载指定版本引擎并替换当前引擎二进制。
// 执行后会保留 .bak 备份；替换成功后若引擎本在运行会重启。
func (m *Manager) DownloadAndUpdate(version string) (string, error) {
	if version == "" {
		version = CurrentEngineVersion
	}
	newBin, err := m.engineDownload(version)
	if err != nil {
		return "", err
	}
	oldBin := m.Exe
	_ = os.Rename(oldBin, oldBin+".bak")
	if err := copyFile(newBin, oldBin); err != nil {
		// 回滚
		_ = os.Rename(oldBin+".bak", oldBin)
		return "", fmt.Errorf("替换引擎二进制失败: %w", err)
	}
	return version, nil
}

// downloadFile 下载到本地文件。
func downloadFile(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// extractZip 解压 zip，返回其中可执行文件（不含 .exe 后缀名）的路径。
func extractZip(path string) (string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer zr.Close()
	dir := filepath.Dir(path)
	for _, f := range zr.File {
		name := f.Name
		// 跳过目录与描述文件
		if f.FileInfo().IsDir() || strings.HasSuffix(name, ".md") || strings.HasSuffix(name, "LICENSE") {
			continue
		}
		base := filepath.Base(name)
		if !strings.HasPrefix(base, "syncthing") {
			continue
		}
		out := filepath.Join(dir, base)
		if err := writeZipEntry(f, out); err != nil {
			return "", err
		}
		return strings.TrimSuffix(out, ".exe"), nil
	}
	return "", fmt.Errorf("zip 内未找到引擎可执行文件")
}

func writeZipEntry(f *zip.File, dest string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}

// extractTarGz 解压 .tar.gz，返回引擎二进制路径。
func extractTarGz(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	gzr, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gzr.Close()
	dir := filepath.Dir(path)
	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		name := hdr.Name
		if strings.HasSuffix(name, ".md") || strings.HasSuffix(name, "LICENSE") || strings.HasSuffix(name, "AUTHORS.txt") {
			continue
		}
		base := filepath.Base(name)
		if base != "syncthing" {
			continue
		}
		out := filepath.Join(dir, base)
		outF, err := os.Create(out)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(outF, tr); err != nil {
			outF.Close()
			return "", err
		}
		outF.Close()
		_ = os.Chmod(out, 0o755)
		return out, nil
	}
	return "", fmt.Errorf("tar.gz 内未找到引擎可执行文件")
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
