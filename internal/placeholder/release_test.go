package placeholder

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClearLocal(t *testing.T) {
	dir := t.TempDir()
	// 造出文件、子目录、.stignore 与 .stfolder 标记
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := []string{"a.txt", "sub/b.bin", StIgnoreFile, ".stfolder"}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := clearLocal(dir); err != nil {
		t.Fatalf("clearLocal 失败: %v", err)
	}
	// 文件/子目录应被删除，.stignore 与 .stfolder 保留
	for _, f := range []string{"a.txt", "sub"} {
		if _, err := os.Stat(filepath.Join(dir, f)); !os.IsNotExist(err) {
			t.Fatalf("%s 应被删除，err=%v", f, err)
		}
	}
	for _, f := range []string{StIgnoreFile, ".stfolder"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("%s 应保留: %v", f, err)
		}
	}
}

func TestClearLocalMissingDir(t *testing.T) {
	if err := clearLocal(filepath.Join(t.TempDir(), "nope")); err != nil {
		t.Fatalf("目录不存在应无错误: %v", err)
	}
}
