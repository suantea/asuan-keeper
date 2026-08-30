//go:build windows

package placeholder

import (
	"golang.org/x/sys/windows"
)

// DiskFree 返回目录所在卷的剩余可用字节数。
func DiskFree(dir string) (uint64, error) {
	var free, total, avail uint64
	utf16Ptr, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return 0, err
	}
	if err := windows.GetDiskFreeSpaceEx(utf16Ptr, &free, &total, &avail); err != nil {
		return 0, err
	}
	return free, nil
}
