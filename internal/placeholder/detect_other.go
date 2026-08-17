//go:build !windows

package placeholder

import "os"

// DriverAvailable 返回当前平台 FUSE 运行时是否可用。
// macOS 需 macFUSE（/Library/Filesystems/macfuse.fs），Linux 需 FUSE（/dev/fuse）。
// 均不存在时返回 false，供上层给出引导提示。
func DriverAvailable() bool {
	for _, p := range []string{
		"/dev/fuse",                                        // Linux
		"/Library/Filesystems/macfuse.fs",                  // macOS macFUSE
		"/Library/Filesystems/macfuse.fs/Contents/Resources", // macOS 兼容路径
	} {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}
