//go:build !windows

package placeholder

import (
	"syscall"
)

// DiskFree 返回目录所在文件系统的剩余可用字节数。
func DiskFree(dir string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, err
	}
	return uint64(st.Bavail) * uint64(st.Bsize), nil
}
