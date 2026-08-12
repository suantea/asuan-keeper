// Package placeholder 实现 P1 占位符层：文件夹级释放 / 水合。
//
// 释放（release）= 仅删除本地实体、不传播到对端：在文件夹写入
// .stignore 的 "(?d)*" 规则（忽略全部条目且忽略删除），再删除本地
// 实体，本地空间释放但对端内容保留。水合（hydrate）= 移除该规则并
// 触发重扫，文件从对端重新拉取。
package placeholder

import (
	"os"
	"path/filepath"
	"strings"
)

// StIgnoreFile 是 Syncthing 文件夹忽略规则文件名。
const StIgnoreFile = ".stignore"

// ReleaseRule 是文件夹级释放规则：匹配全部条目并忽略删除
// （本地删除不传播、对端内容不受影响、本地不重新拉回）。
const ReleaseRule = "(?d)*"

// ReadStIgnore 读取文件夹下的忽略规则（按行，去空行与 // 注释）。
func ReadStIgnore(dir string) ([]string, error) {
	b, err := os.ReadFile(filepath.Join(dir, StIgnoreFile))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var rules []string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		rules = append(rules, line)
	}
	return rules, nil
}

// WriteStIgnore 将规则写入文件夹的 .stignore（覆盖）。
func WriteStIgnore(dir string, rules []string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	content := strings.Join(rules, "\n")
	if content != "" {
		content += "\n"
	}
	return os.WriteFile(filepath.Join(dir, StIgnoreFile), []byte(content), 0o644)
}

// IsReleased 判断文件夹当前是否处于释放状态（存在释放规则）。
func IsReleased(dir string) (bool, error) {
	rules, err := ReadStIgnore(dir)
	if err != nil {
		return false, err
	}
	for _, r := range rules {
		if r == ReleaseRule {
			return true, nil
		}
	}
	return false, nil
}
