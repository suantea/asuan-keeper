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
