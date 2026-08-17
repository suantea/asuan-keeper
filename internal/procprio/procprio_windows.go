//go:build windows

package procprio

import "golang.org/x/sys/windows"

// SetBelowNormal 将指定 PID 的进程优先级设为 Below Normal。
// 供 sidecar 引擎等后台任务使用，避免抢占前台应用的 CPU/IO。
// 失败时静默忽略（非致命：仅影响调度优先级）。
func SetBelowNormal(pid int) {
	if pid <= 0 {
		return
	}
	h, err := windows.OpenProcess(windows.PROCESS_SET_INFORMATION, false, uint32(pid))
	if err != nil {
		return
	}
	defer windows.CloseHandle(h)
	_ = windows.SetPriorityClass(h, windows.BELOW_NORMAL_PRIORITY_CLASS)
}
