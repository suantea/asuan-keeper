package placeholder

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestWaitPathExists(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "水合.txt")
	// 文件尚不存在：短超时应报错
	if err := waitPathExists(target, 500*time.Millisecond); err == nil {
		t.Fatal("不存在的路径应超时返回错误")
	}
	// 延迟创建后应等到
	go func() {
		time.Sleep(200 * time.Millisecond)
		_ = os.WriteFile(target, []byte("x"), 0o644)
	}()
	if err := waitPathExists(target, 5*time.Second); err != nil {
		t.Fatalf("应等到文件出现: %v", err)
	}
	// 已存在立即返回
	if err := waitPathExists(target, time.Second); err != nil {
		t.Fatalf("已存在的路径应立即返回: %v", err)
	}
}
