//go:build windows

package firewall

import (
	"fmt"
	"os/exec"
	"strings"
)

// Add 添加入站 TCP 放行规则(仅端口,不暴露程序路径)。
// 需要管理员权限:netsh 失败时提示以管理员运行。
func Add(port int) error {
	if port <= 0 || port > 65535 {
		return fmt.Errorf("非法端口: %d", port)
	}
	name := RuleName(port)
	cmd := exec.Command("netsh", "advfirewall", "firewall", "add", "rule",
		"name="+name, "dir=in", "action=allow", "protocol=TCP", "localport="+fmt.Sprint(port))
	if out, err := cmd.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		return fmt.Errorf("添加防火墙规则失败(请以管理员身份运行): %w %s", err, msg)
	}
	return nil
}

// Remove 移除放行规则。
func Remove(port int) error {
	name := RuleName(port)
	cmd := exec.Command("netsh", "advfirewall", "firewall", "delete", "rule", "name="+name)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("删除防火墙规则失败(请以管理员身份运行): %w %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Status 返回规则是否存在。
func Status(port int) (bool, error) {
	name := RuleName(port)
	cmd := exec.Command("netsh", "advfirewall", "firewall", "show", "rule", "name="+name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// 规则不存在时 netsh 返回非零退出码。
		return false, nil
	}
	return strings.Contains(string(out), name), nil
}
