//go:build !windows

// Package procprio 设置子进程调度优先级（Windows 实现，其他平台 no-op）。
package procprio

// SetBelowNormal 非 Windows 平台占位：不做调整。
func SetBelowNormal(pid int) {}
