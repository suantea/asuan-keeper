//go:build !windows

package firewall

import "fmt"

// IsAdmin 非 Windows 平台无管理员概念（UID 0 视为管理员）。
func IsAdmin() bool {
	return false
}

// Add 非 Windows 平台占位:Linux(UFW)/macOS 由部署脚本或系统防火墙管理,
// 返回明确提示而非静默失败。networks 为可选 IP 白名单（忽略）。
func Add(port int, networks []string) error {
	return fmt.Errorf("当前平台不支持自动放行防火墙,请手动放行 TCP 端口 %d", port)
}

// Remove 非 Windows 平台占位。
func Remove(port int) error {
	return fmt.Errorf("当前平台不支持自动管理防火墙规则")
}

// Status 非 Windows 平台占位:视为未配置(避免误报)。
func Status(port int) (bool, error) {
	return false, nil
}
