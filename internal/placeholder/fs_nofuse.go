//go:build !cgofuse

package placeholder

import (
	"errors"
	"strings"
)

// 无 cgofuse 构建下的占位符虚拟层桩。
//
// 正式发行版以 `-tags cgofuse` 构建真实实现（见 scripts/build-dist.sh 与
// deploy/MAC.md），需要 WinFsp / macFUSE / FUSE 头文件。提供本桩的原因：
// 让 `go build ./...` 与 `go test ./...` 在没有 FUSE 头文件的开发机（例如
// 未安装 macFUSE 的 macOS）和 CI 上直接通过——除"挂载虚拟层"以外的全部
// 功能（release/hydrate/列表/缓存）都不依赖 FUSE。

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

// ErrNoFuse 表示当前二进制构建时未启用 cgofuse，虚拟层不可用。
var ErrNoFuse = errors.New("本二进制未启用虚拟层（需以 -tags cgofuse 构建，且安装 WinFsp/macFUSE/FUSE）")

// PlaceholderFS 是占位符虚拟层的无 FUSE 桩：API 与真实实现一致，
// 但 Mount 永远返回 ErrNoFuse。
type PlaceholderFS struct {
	lister  Lister
	root    string
	hydrate func(rel string) error
	resolve func(rel string) (string, error)
}

// NewPlaceholderFS 创建占位符虚拟层桩（签名与真实实现一致）。
func NewPlaceholderFS(lister Lister, root string, hydrate func(rel string) error) *PlaceholderFS {
	return &PlaceholderFS{lister: lister, root: root, hydrate: hydrate}
}

// SetResolver 设置虚拟相对路径 → 本地真实绝对路径的映射函数（桩中仅保存）。
func (fs *PlaceholderFS) SetResolver(fn func(rel string) (string, error)) *PlaceholderFS {
	fs.resolve = fn
	return fs
}

// Mount 在无 cgofuse 构建下不可用。
func (fs *PlaceholderFS) Mount(string) error { return ErrNoFuse }

// rel 去掉前导 "/"（与真实实现保持一致，供同包测试使用）。
func rel(path string) string {
	return strings.TrimPrefix(path, "/")
}
