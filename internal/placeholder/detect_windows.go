//go:build windows

package placeholder

import "os"

// DriverAvailable 返回 Windows 下 WinFsp 驱动是否已安装。
// 检测驱动文件 winfsp.sys 是否存在（WinFsp 安装即注册该驱动）。
// 未安装时占位符虚拟层无法挂载，返回 false 供上层给出引导提示。
func DriverAvailable() bool {
	for _, p := range []string{
		`C:\Windows\System32\drivers\winfsp.sys`,
		`C:\Windows\SysWOW64\drivers\winfsp.sys`,
	} {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}
