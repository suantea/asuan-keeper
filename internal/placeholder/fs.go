//go:build cgofuse

package placeholder

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/winfsp/cgofuse/fuse"
)

// 占位符虚拟层（P1）：基于 cgofuse 的只读虚拟文件系统。
//
// 挂载后，已释放文件夹中对端仍拥有的文件以"占位条目"的形式可见；
// 打开/读取占位文件时自动触发水合回调（从对端拉取），随后从本地
// 真实路径读取内容——即"双击水合"。
//
// 运行时要求：Windows 需安装 WinFsp（https://winfsp.dev/），
// macOS 需 macFUSE，Linux 需 FUSE；仅编译本包不需要这些驱动。

// Entry 是占位目录中的一个条目（对端索引中的文件/目录）。
type Entry struct {
	Name  string
	IsDir bool
	Size  int64
}

// Lister 提供占位目录的条目列表（来源：syncthing 全局索引或本地占位清单）。
type Lister interface {
	// List 返回目录 relDir 下的条目；relDir 为空串表示虚拟层根目录。
	// 返回的条目 Name 为纯文件名（不含路径）。
	List(relDir string) ([]Entry, error)
}

// PlaceholderFS 是占位符虚拟层文件系统。
//
// 数据流：Getattr/Readdir 从 Lister 取对端索引；Open/Read 先触发
// Hydrate 回调把文件从对端拉回本地真实目录，再从本地读取。
type PlaceholderFS struct {
	fuse.FileSystemBase

	lister  Lister
	root    string                 // 本地真实目录（水合后文件落盘位置）
	hydrate func(rel string) error // 水合回调：将 rel 文件从对端拉回本地
	resolve func(rel string) (string, error) // 虚拟相对路径 → 本地真实绝对路径
	flights *hydrateFlights                  // 并发打开同一文件的水合去重
}

// NewPlaceholderFS 创建占位符虚拟层。
// root 是本地真实目录；hydrate 可为 nil（此时 Read 直接尝试读本地，不拉取）。
// 虚拟层根目录下为多个已释放文件夹时，应通过 SetResolver 提供
// 虚拟路径 → 真实路径的映射（否则按 root 直接拼接）。
func NewPlaceholderFS(lister Lister, root string, hydrate func(rel string) error) *PlaceholderFS {
	return &PlaceholderFS{lister: lister, root: root, hydrate: hydrate, flights: newHydrateFlights()}
}

// SetResolver 设置虚拟相对路径 → 本地真实绝对路径的映射函数。
func (fs *PlaceholderFS) SetResolver(fn func(rel string) (string, error)) *PlaceholderFS {
	fs.resolve = fn
	return fs
}

// resolvePath 把虚拟层相对路径映射为本地真实路径。
func (fs *PlaceholderFS) resolvePath(virtRel string) (string, error) {
	if fs.resolve != nil {
		return fs.resolve(virtRel)
	}
	return filepath.Join(fs.root, filepath.FromSlash(virtRel)), nil
}

// rel 去掉前导 "/" 得到相对路径。
func rel(path string) string {
	return strings.TrimPrefix(path, "/")
}

// Statfs 返回虚拟层基本统计。
func (fs *PlaceholderFS) Statfs(path string, stat *fuse.Statfs_t) int {
	stat.Bsize = 4096
	stat.Frsize = 4096
	stat.Namemax = 255
	return 0
}

// Getattr 返回路径属性：根与目录为 S_IFDIR，占位文件为 S_IFREG（只读）。
func (fs *PlaceholderFS) Getattr(path string, stat *fuse.Stat_t, fh uint64) int {
	if path == "/" {
		stat.Mode = fuse.S_IFDIR | 0o555
		stat.Nlink = 2
		return 0
	}
	dir, base := splitVirt(rel(path))
	entries, err := fs.lister.List(dir)
	if err != nil {
		return -fuse.EIO
	}
	for _, e := range entries {
		if e.Name != base {
			continue
		}
		if e.IsDir {
			stat.Mode = fuse.S_IFDIR | 0o555
			stat.Nlink = 2
		} else {
			stat.Mode = fuse.S_IFREG | 0o444
			stat.Nlink = 1
			stat.Size = e.Size
		}
		return 0
	}
	return -fuse.ENOENT
}

// Readdir 列出目录条目：根目录与子目录均来自 Lister，附 "." 与 ".."。
func (fs *PlaceholderFS) Readdir(path string,
	fill func(name string, stat *fuse.Stat_t, ofst int64) bool,
	ofst int64, fh uint64) int {

	entries, err := fs.lister.List(rel(path))
	if err != nil {
		return -fuse.EIO
	}
	if !fill(".", nil, 0) || !fill("..", nil, 0) {
		return 0
	}
	for _, e := range entries {
		if !fill(e.Name, nil, 0) {
			break
		}
	}
	return 0
}

// Open 仅允许只读打开（占位符虚拟层不可写）。打开即触发水合（同一文件
// 并发去重），并按对端索引的大小等待文件完整落盘，保证 Read 时本地已有
// 完整实体。
func (fs *PlaceholderFS) Open(path string, flags int) (int, uint64) {
	if flags&fuse.O_ACCMODE != fuse.O_RDONLY {
		return -fuse.EACCES, ^uint64(0)
	}
	if fs.hydrate != nil {
		relPath := rel(path)
		real, err := fs.resolvePath(relPath)
		if err != nil {
			return -fuse.EIO, ^uint64(0)
		}
		wantSize := entrySizeFromLister(fs.lister, relPath)
		if err := fs.flights.do(real, func() error {
			if err := fs.hydrate(relPath); err != nil {
				return err
			}
			return waitFileReady(real, wantSize, 30*time.Second)
		}); err != nil {
			return -fuse.EIO, ^uint64(0)
		}
	}
	return 0, 0
}

// Read 从本地真实路径读取（Open 已触发水合）。
func (fs *PlaceholderFS) Read(path string, buff []byte, ofst int64, fh uint64) int {
	real, err := fs.resolvePath(rel(path))
	if err != nil {
		return -fuse.EIO
	}
	f, err := os.Open(real)
	if err != nil {
		return -fuse.EIO
	}
	defer f.Close()
	n, err := f.ReadAt(buff, ofst)
	if err != nil && !errors.Is(err, io.EOF) {
		return -fuse.EIO
	}
	return n
}

// Mount 挂载占位虚拟层到指定挂载点（阻塞运行，需 WinFsp/macFUSE/FUSE）。
// 返回文件系统宿主；调用方需保持运行直到取消。
//
// 平台差异：
//   - Windows/WinFsp：挂载点必须"预先不存在"——由文件系统在挂载时
//     自动创建、卸载时删除；预先存在的目录（即使为空）会导致
//     "mount point in use" 失败。若挂载点已存在且为空目录，此处先
//     安全移除后再挂载；非空目录则报错。
//   - macOS/Linux（macFUSE/FUSE）：挂载点必须"预先存在"（空目录），
//     不存在则自动创建，已存在的非空目录同样报错。
func (fs *PlaceholderFS) Mount(mountpoint string) error {
	if fi, err := os.Stat(mountpoint); err == nil {
		if !fi.IsDir() {
			return fmt.Errorf("挂载点必须是目录: %s", mountpoint)
		}
		entries, err := os.ReadDir(mountpoint)
		if err != nil {
			return fmt.Errorf("读取挂载点失败: %w", err)
		}
		if len(entries) > 0 {
			return fmt.Errorf("挂载点目录必须为空: %s", mountpoint)
		}
		// WinFsp 要求挂载点不存在（自动创建）；macFUSE/FUSE 要求存在。
		// 为空目录时：Windows 移除后由 WinFsp 重建，其他平台保留供挂载。
		if runtime.GOOS == "windows" {
			if err := os.Remove(mountpoint); err != nil {
				return fmt.Errorf("清理空挂载点失败: %w", err)
			}
		}
	} else if os.IsNotExist(err) {
		// macFUSE/FUSE 要求挂载点已存在：自动创建空目录。
		if runtime.GOOS != "windows" {
			if err := os.MkdirAll(mountpoint, 0o755); err != nil {
				return fmt.Errorf("创建挂载点失败: %w", err)
			}
		}
	} else {
		return fmt.Errorf("检查挂载点失败: %w", err)
	}
	host := fuse.NewFileSystemHost(fs)
	if !host.Mount(mountpoint, nil) {
		return fmt.Errorf("挂载失败（请确认已安装 WinFsp / macFUSE / FUSE）")
	}
	host.Unmount()
	return nil
}
