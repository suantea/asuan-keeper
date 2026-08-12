package placeholder

import (
	"path/filepath"
	"strings"

	"atomgit.com/suantea/asuan-keeper/internal/config"
	"atomgit.com/suantea/asuan-keeper/internal/syncthing"
)

// ReleaseLister 是占位符虚拟层的数据源：根目录列出所有已释放文件夹，
// 子目录条目来自 syncthing 全局索引（对端内容视图，db/browse）。
// 挂载后用户看到的就是"释放文件夹里对端有哪些文件"。
type ReleaseLister struct {
	cfg *config.Config
	m   *syncthing.Manager
}

// NewReleaseLister 创建基于 syncthing 全局索引的占位符 Lister。
func NewReleaseLister(cfg *config.Config, m *syncthing.Manager) *ReleaseLister {
	return &ReleaseLister{cfg: cfg, m: m}
}

// List 返回 relDir 下的条目。
//
// relDir 为空串表示虚拟层根：列出所有处于释放状态的 sync 文件夹
// （以 label 为目录名）。非根路径的首段是文件夹 label，其后转发给
// syncthing 全局索引。
func (l *ReleaseLister) List(relDir string) ([]Entry, error) {
	if relDir == "" {
		var out []Entry
		for _, f := range l.cfg.SyncedFolders() {
			released, err := IsReleased(f.Path)
			if err != nil || !released {
				continue
			}
			name := f.Label
			if name == "" {
				name = f.ID
			}
			out = append(out, Entry{Name: name, IsDir: true})
		}
		return out, nil
	}
	parts := strings.SplitN(relDir, "/", 2)
	label, sub := parts[0], ""
	if len(parts) > 1 {
		sub = parts[1]
	}
	f, err := l.folderByLabel(label)
	if err != nil {
		return nil, err
	}
	items, err := l.m.Browse(f.ID, sub)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(items))
	for _, it := range items {
		out = append(out, Entry{Name: it.Name, IsDir: it.Type == "DIRECTORY", Size: it.Size})
	}
	return out, nil
}

// folderByLabel 通过 label（或回退 id）查找 sync 文件夹。
func (l *ReleaseLister) folderByLabel(label string) (*config.Folder, error) {
	for i := range l.cfg.Folders {
		f := &l.cfg.Folders[i]
		if f.Policy != config.PolicySync {
			continue
		}
		if f.Label == label || f.ID == label {
			return f, nil
		}
	}
	return nil, ErrFolderNotFound(label)
}

// FolderIDOf 从虚拟层相对路径（首段为文件夹 label/id）解析出文件夹 ID，
// 供水合回调把"打开哪个占位文件"映射到"水合哪个文件夹"。
func FolderIDOf(l *ReleaseLister, relPath string) string {
	if l == nil {
		return ""
	}
	parts := strings.SplitN(relPath, "/", 2)
	f, err := l.folderByLabel(parts[0])
	if err != nil {
		return parts[0] // 找不到时退回首段，让下游报错更明确
	}
	return f.ID
}

// SplitVirt 把虚拟层相对路径拆成 (folderID, 文件夹内相对路径)。
// 首段是文件夹 label/id，其后是文件夹内子路径（可能为空）。
func (l *ReleaseLister) SplitVirt(virtRel string) (folderID, subRel string) {
	parts := strings.SplitN(virtRel, "/", 2)
	f, err := l.folderByLabel(parts[0])
	if err != nil {
		return parts[0], ""
	}
	sub := ""
	if len(parts) > 1 {
		sub = parts[1]
	}
	return f.ID, sub
}

// Resolve 把虚拟层相对路径（首段为文件夹 label/id）映射为本地真实绝对路径，
// 供 PlaceholderFS.Read 从水合后的本地实体读取。
func (l *ReleaseLister) Resolve(virtRel string) (string, error) {
	parts := strings.SplitN(virtRel, "/", 2)
	f, err := l.folderByLabel(parts[0])
	if err != nil {
		return "", err
	}
	sub := ""
	if len(parts) > 1 {
		sub = parts[1]
	}
	real := filepath.Join(f.Path, filepath.FromSlash(sub))
	return real, nil
}
