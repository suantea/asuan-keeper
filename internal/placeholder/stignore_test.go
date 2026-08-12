package placeholder

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStIgnoreReadWrite(t *testing.T) {
	dir := t.TempDir()
	if _, err := ReadStIgnore(dir); err != nil {
		t.Fatalf("空目录读取应无错误: %v", err)
	}
	rules := []string{ReleaseRule, "*.tmp"}
	if err := WriteStIgnore(dir, rules); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	got, err := ReadStIgnore(dir)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if len(got) != 2 || got[0] != ReleaseRule || got[1] != "*.tmp" {
		t.Fatalf("规则读写不一致: %v", got)
	}
}

func TestStIgnoreSkipsCommentsAndBlank(t *testing.T) {
	dir := t.TempDir()
	content := "// 注释行\n\n(?d)*\n// 另一条注释\n*.log\n"
	if err := os.WriteFile(filepath.Join(dir, StIgnoreFile), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadStIgnore(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{ReleaseRule, "*.log"}
	if len(got) != len(want) {
		t.Fatalf("规则数不符: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("规则不符: got %v want %v", got, want)
		}
	}
}

func TestIsReleased(t *testing.T) {
	dir := t.TempDir()
	rel, err := IsReleased(dir)
	if err != nil {
		t.Fatal(err)
	}
	if rel {
		t.Fatal("空文件夹不应视为已释放")
	}
	if err := WriteStIgnore(dir, []string{ReleaseRule}); err != nil {
		t.Fatal(err)
	}
	rel, err = IsReleased(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !rel {
		t.Fatal("含 (?d)* 规则应视为已释放")
	}
	// 移除释放规则后不再视为释放
	if err := WriteStIgnore(dir, []string{"*.tmp"}); err != nil {
		t.Fatal(err)
	}
	rel, _ = IsReleased(dir)
	if rel {
		t.Fatal("移除释放规则后不应视为已释放")
	}
}

func TestPathRules(t *testing.T) {
	if got := PathRules("sub/file.txt", false); len(got) != 1 || got[0] != "(?d)/sub/file.txt" {
		t.Fatalf("文件规则不符: %v", got)
	}
	if got := PathRules("sub/dir", true); len(got) != 2 || got[0] != "(?d)/sub/dir" || got[1] != "(?d)/sub/dir/**" {
		t.Fatalf("目录规则不符: %v", got)
	}
	if got := PathRules("", false); got != nil {
		t.Fatalf("空路径应返回 nil: %v", got)
	}
	// 反斜杠与多余 ./ 规范化
	if got := PathRules("./sub\\dir", true); got[0] != "(?d)/sub/dir" {
		t.Fatalf("路径规范化不符: %v", got)
	}
}

func TestAddRemoveRules(t *testing.T) {
	dir := t.TempDir()
	if err := AddRules(dir, []string{"(?d)/a.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := AddRules(dir, []string{"(?d)/a.txt", "(?d)/b/"}); err != nil {
		t.Fatal(err)
	}
	rules, _ := ReadStIgnore(dir)
	if len(rules) != 2 {
		t.Fatalf("去重后规则数不符: %v", rules)
	}
	// IsPathReleased 单文件命中
	if rel, _ := IsPathReleased(dir, "a.txt", false); !rel {
		t.Fatal("a.txt 应视为已释放")
	}
	if rel, _ := IsPathReleased(dir, "b", true); !rel {
		t.Fatal("b 目录应视为已释放")
	}
	if rel, _ := IsPathReleased(dir, "c.txt", false); rel {
		t.Fatal("c.txt 不应视为已释放")
	}
	// 移除后不再命中
	if err := RemoveRules(dir, []string{"(?d)/a.txt"}); err != nil {
		t.Fatal(err)
	}
	if rel, _ := IsPathReleased(dir, "a.txt", false); rel {
		t.Fatal("移除后 a.txt 不应视为已释放")
	}
	// ReleasedPaths 列出单路径（不含文件夹级）
	if err := AddRules(dir, []string{ReleaseRule}); err != nil {
		t.Fatal(err)
	}
	paths, _ := ReleasedPaths(dir)
	if len(paths) != 1 || paths[0] != "b/" {
		t.Fatalf("ReleasedPaths 不符: %v", paths)
	}
}
