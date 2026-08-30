//go:build cgofuse

package placeholder

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/winfsp/cgofuse/fuse"
)

// memLister 是内存版 Lister，模拟对端索引。
type memLister struct {
	entries map[string][]Entry
}

func (m *memLister) List(relDir string) ([]Entry, error) {
	return m.entries[relDir], nil
}

func TestFSGetattr(t *testing.T) {
	fs := NewPlaceholderFS(&memLister{entries: map[string][]Entry{
		"":     {{Name: "docs", IsDir: true}, {Name: "a.txt", Size: 42}},
		"docs": {{Name: "b.bin", Size: 7}},
	}}, t.TempDir(), nil)

	var st fuse.Stat_t
	if rc := fs.Getattr("/", &st, 0); rc != 0 {
		t.Fatalf("根目录 Getattr: %d", rc)
	}
	if st.Mode&fuse.S_IFMT != fuse.S_IFDIR {
		t.Fatalf("根应为目录, mode=%o", st.Mode)
	}
	if rc := fs.Getattr("/a.txt", &st, 0); rc != 0 {
		t.Fatalf("文件 Getattr: %d", rc)
	}
	if st.Size != 42 || st.Mode&fuse.S_IFMT != fuse.S_IFREG {
		t.Fatalf("文件属性不符: size=%d mode=%o", st.Size, st.Mode)
	}
	if rc := fs.Getattr("/docs", &st, 0); rc != 0 {
		t.Fatalf("子目录 Getattr: %d", rc)
	}
	if rc := fs.Getattr("/nope", &st, 0); rc != -fuse.ENOENT {
		t.Fatalf("不存在条目应返回 ENOENT: %d", rc)
	}
}

func TestFSReaddir(t *testing.T) {
	fs := NewPlaceholderFS(&memLister{entries: map[string][]Entry{
		"": {{Name: "a.txt", Size: 1}},
	}}, t.TempDir(), nil)

	var names []string
	rc := fs.Readdir("/", func(name string, _ *fuse.Stat_t, _ int64) bool {
		names = append(names, name)
		return true
	}, 0, 0)
	if rc != 0 {
		t.Fatalf("Readdir: %d", rc)
	}
	want := []string{".", "..", "a.txt"}
	if len(names) != len(want) {
		t.Fatalf("条目数不符: %v", names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("条目不符: %v", names)
		}
	}
}

func TestFSReadHydrate(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(real, []byte("hello asuan"), 0o644); err != nil {
		t.Fatal(err)
	}
	hydrated := false
	fs := NewPlaceholderFS(&memLister{entries: map[string][]Entry{
		"": {{Name: "hello.txt", Size: 11}},
	}}, dir, func(rel string) error {
		hydrated = true
		return nil
	})
	if rc, _ := fs.Open("/hello.txt", fuse.O_RDONLY); rc != 0 {
		t.Fatalf("Open: %d", rc)
	}
	if !hydrated {
		t.Fatal("Open 应触发水合回调")
	}
	buf := make([]byte, 16)
	n := fs.Read("/hello.txt", buf, 0, 0)
	if n != 11 || string(buf[:n]) != "hello asuan" {
		t.Fatalf("Read 结果不符: n=%d data=%q", n, buf[:n])
	}
}

func TestFSOpenWriteDenied(t *testing.T) {
	fs := NewPlaceholderFS(&memLister{entries: map[string][]Entry{
		"": {{Name: "a.txt"}},
	}}, t.TempDir(), nil)
	if rc, _ := fs.Open("/a.txt", fuse.O_WRONLY); rc != -fuse.EACCES {
		t.Fatalf("写打开应被拒绝: %d", rc)
	}
}
