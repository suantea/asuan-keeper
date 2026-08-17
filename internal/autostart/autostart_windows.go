//go:build windows

// Package autostart 管理开机自启（Windows：HKCU Run 注册表，免管理员）。
package autostart

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

// RunKey 是 HKCU 下的开机启动注册表键。
const RunKey = `Software\Microsoft\Windows\CurrentVersion\Run`

// ValueName 是 asuan 自启项的名称。
const ValueName = "asuan"

// exePath 返回当前可执行文件的绝对路径（供自启命令构造）。
func exePath() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Abs(p)
}

// Status 返回自启是否已注册。
func Status() (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, RunKey, registry.QUERY_VALUE)
	if err != nil {
		if err == registry.ErrNotExist {
			return false, nil
		}
		return false, err
	}
	defer k.Close()
	if _, _, err := k.GetStringValue(ValueName); err != nil {
		if err == registry.ErrNotExist {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Enable 注册开机自启（HKCU Run，无需管理员）。
// 用 wscript 隐藏窗口启动 asuan run，避免开机弹控制台黑窗。
func Enable() error {
	exe, err := exePath()
	if err != nil {
		return err
	}
	// wscript 启动命令：run hidden (0)，不等待返回。
	cmd := fmt.Sprintf(`wscript.exe "//nologo" "//e:vbscript" "%s"`, vbsPath(exe))
	k, _, err := registry.CreateKey(registry.CURRENT_USER, RunKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("打开注册表 Run 键失败: %w", err)
	}
	defer k.Close()
	if err := k.SetStringValue(ValueName, cmd); err != nil {
		return fmt.Errorf("写入自启项失败: %w", err)
	}
	// 生成隐藏启动器 vbs（与 asuan.exe 同目录）。
	if err := writeLauncherVBS(exe); err != nil {
		return fmt.Errorf("生成隐藏启动器失败: %w", err)
	}
	return nil
}

// Disable 移除开机自启。
func Disable() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, RunKey, registry.SET_VALUE)
	if err != nil {
		if err == registry.ErrNotExist {
			return nil
		}
		return err
	}
	defer k.Close()
	if err := k.DeleteValue(ValueName); err != nil {
		if err == registry.ErrNotExist {
			return nil
		}
		return err
	}
	return nil
}

// vbsPath 返回与 asuan.exe 同目录的隐藏启动器路径。
func vbsPath(exe string) string {
	return filepath.Join(filepath.Dir(exe), "asuan-autostart.vbs")
}

// writeLauncherVBS 生成隐藏窗口启动器：wscript 隐藏运行 asuan run。
func writeLauncherVBS(exe string) error {
	content := `' asuan 开机自启隐藏启动器（wscript 运行，无控制台窗口）
Set sh = CreateObject("WScript.Shell")
sh.Run """" & "` + exe + `" & """ run", 0, False
`
	return os.WriteFile(vbsPath(exe), []byte(content), 0o600)
}
