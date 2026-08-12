// Package placeholder 实现占位符层：文件夹级 / 单文件级释放与水合。
//
// 释放（release）= 仅删除本地实体、不传播到对端：在文件夹写入
// .stignore 的 "(?d)" 忽略删除规则，再删除本地实体，本地空间释放
// 但对端内容保留。水合（hydrate）= 移除对应规则并触发重扫，内容
// 从对端重新拉取。
//
// 规则粒度：
//   - 文件夹级：`(?d)*`（忽略全部条目且忽略删除）
//   - 单路径级：`(?d)/<relpath>`（文件）或 `(?d)/<dir>` + `(?d)/<dir>/**`（目录）
package placeholder

import (
	"os"
	"path"
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

// IsReleased 判断文件夹当前是否处于文件夹级释放状态（存在 (?d)* 规则）。
func IsReleased(dir string) (bool, error) {
	rules, err := ReadStIgnore(dir)
	if err != nil {
		return false, err
	}
	return hasRule(rules, ReleaseRule), nil
}

// PathRules 生成单路径释放规则列表：文件一条 `(?d)/path`；
// 目录两条 `(?d)/dir` + `(?d)/dir/**`（匹配目录本身及其下全部内容）。
// relPath 为空返回 nil。
func PathRules(relPath string, isDir bool) []string {
	relPath = strings.Trim(cleanRel(relPath), "/")
	if relPath == "" {
		return nil
	}
	if !isDir {
		return []string{"(?d)/" + relPath}
	}
	return []string{"(?d)/" + relPath, "(?d)/" + relPath + "/**"}
}

// IsPathReleased 判断某个相对路径是否已被释放（存在对应 (?d) 规则）。
// 目录规则接受 `(?d)/dir`、`(?d)/dir/`、`(?d)/dir/**` 任一写法；
// relPath 为空表示文件夹级（(?d)*）。
func IsPathReleased(dir, relPath string, isDir bool) (bool, error) {
	rules, err := ReadStIgnore(dir)
	if err != nil {
		return false, err
	}
	relPath = strings.Trim(cleanRel(relPath), "/")
	if relPath == "" {
		return hasRule(rules, ReleaseRule), nil
	}
	candidates := []string{"(?d)/" + relPath}
	if isDir {
		candidates = append(candidates, "(?d)/"+relPath+"/", "(?d)/"+relPath+"/**")
	}
	for _, c := range candidates {
		if hasRule(rules, c) {
			return true, nil
		}
	}
	return false, nil
}

// AddRules 追加规则（已存在则跳过），保持原有规则顺序。
func AddRules(dir string, add []string) error {
	rules, err := ReadStIgnore(dir)
	if err != nil {
		return err
	}
	for _, a := range add {
		if !hasRule(rules, a) {
			rules = append(rules, a)
		}
	}
	return WriteStIgnore(dir, rules)
}

// RemoveRules 移除指定规则（文件夹级 (?d)* 或单路径 (?d)/...）。
func RemoveRules(dir string, remove []string) error {
	rules, err := ReadStIgnore(dir)
	if err != nil {
		return err
	}
	rem := map[string]bool{}
	for _, r := range remove {
		rem[r] = true
	}
	kept := rules[:0]
	for _, r := range rules {
		if !rem[r] {
			kept = append(kept, r)
		}
	}
	return WriteStIgnore(dir, kept)
}

// ReleasedPaths 列出文件夹下所有已释放的单路径（去掉 (?d)/ 前缀），
// 供 UI/CLI 展示。不包含文件夹级 (?d)*。
func ReleasedPaths(dir string) ([]string, error) {
	rules, err := ReadStIgnore(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, r := range rules {
		if r == ReleaseRule {
			continue
		}
		if strings.HasPrefix(r, "(?d)/") {
			out = append(out, strings.TrimPrefix(r, "(?d)/"))
		}
	}
	return out, nil
}

func hasRule(rules []string, rule string) bool {
	for _, r := range rules {
		if r == rule {
			return true
		}
	}
	return false
}

// cleanRel 规范化相对路径：正斜杠、去 ./、去尾部 /；空串保持不变。
func cleanRel(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	return path.Clean(p)
}
