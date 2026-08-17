//go:build !windows

// Package autostart 管理开机自启（非 Windows 平台占位）。
package autostart

import "fmt"

// Status 非 Windows 平台返回未启用。
func Status() (bool, error) {
	return false, nil
}

// Enable 非 Windows 平台返回明确提示（由系统服务/launchd 管理）。
func Enable() error {
	return fmt.Errorf("当前平台不支持自动注册开机自启，请使用系统服务（systemd/launchd）")
}

// Disable 非 Windows 平台占位。
func Disable() error {
	return fmt.Errorf("当前平台不支持自动管理开机自启")
}
