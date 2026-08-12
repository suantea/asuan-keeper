// Package firewall 管理系统防火墙放行规则,避免 sidecar(syncthing)
// 首次监听端口时弹出系统防火墙对话框,暴露进程与端口。
//
// 隐蔽性设计:规则名使用中性名称(asuan-sync),且按"端口"而非
// "程序路径"放行——不暴露 syncthing.exe 位置;只放行 stealth.tcp_port
// 单个 TCP 端口,范围最小。
package firewall

import "fmt"

// RuleName 返回放行规则名(中性,不暴露 syncthing)。
func RuleName(port int) string {
	return fmt.Sprintf("asuan-sync-%d", port)
}
