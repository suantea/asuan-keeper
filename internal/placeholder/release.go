package placeholder

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"atomgit.com/suantea/asuan-keeper/internal/config"
	"atomgit.com/suantea/asuan-keeper/internal/syncthing"
)

// Release 释放文件夹：仅删本地实体不传播。
//
// 顺序关键：先写入 "(?d)*" 忽略规则，再删除本地实体，最后触发重扫——
// 引擎扫描时把删除视为"被忽略的删除"（(?d) 语义），不会传播到对端，
// 本地空间释放而对端内容保留。
func Release(cfg *config.Config, m *syncthing.Manager, folderID string) error {
	f, err := findFolder(cfg, folderID)
	if err != nil {
		return err
	}
	if f.Policy != config.PolicySync {
		return fmt.Errorf("文件夹 %q 策略为 %s，仅 sync 策略可释放", f.ID, f.Policy)
	}
	if err := WriteStIgnore(f.Path, []string{ReleaseRule}); err != nil {
		return fmt.Errorf("写入 .stignore 失败: %w", err)
	}
	if err := clearLocal(f.Path); err != nil {
		return fmt.Errorf("删除本地实体失败: %w", err)
	}
	return m.Scan(folderID)
}

// Hydrate 水合文件夹：移除释放规则，让引擎从对端重新拉取内容。
// timeout 内未同步完成则返回错误。
func Hydrate(cfg *config.Config, m *syncthing.Manager, folderID string, timeout time.Duration) error {
	f, err := findFolder(cfg, folderID)
	if err != nil {
		return err
	}
	rules, err := ReadStIgnore(f.Path)
	if err != nil {
		return err
	}
	kept := make([]string, 0, len(rules))
	for _, r := range rules {
		if r != ReleaseRule {
			kept = append(kept, r)
		}
	}
	if err := WriteStIgnore(f.Path, kept); err != nil {
		return fmt.Errorf("写入 .stignore 失败: %w", err)
	}
	if err := m.Scan(folderID); err != nil {
		return err
	}
	return m.WaitFolderSynced(folderID, timeout)
}

// ErrFolderNotFound 表示配置中不存在指定文件夹。
type ErrFolderNotFound string

func (e ErrFolderNotFound) Error() string {
	return fmt.Sprintf("配置中未找到文件夹 %q（先编辑 asuan.json 的 folders）", string(e))
}

// findFolder 按 id 在配置中查找文件夹。
func findFolder(cfg *config.Config, folderID string) (*config.Folder, error) {
	for i := range cfg.Folders {
		if cfg.Folders[i].ID == folderID {
			return &cfg.Folders[i], nil
		}
	}
	return nil, ErrFolderNotFound(folderID)
}

// clearLocal 删除文件夹路径下的全部实体，保留 .stignore 与 .stfolder
// （syncthing 文件夹标记，删掉会触发 "folder marker missing"）。
func clearLocal(dir string) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name() == StIgnoreFile || e.Name() == ".stfolder" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}
