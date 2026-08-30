package placeholder

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// 并发打开同一文件必须只触发一次水合：文件管理器的缩略图/预览会并发读
// 同一文件，不去重会为同一文件起多个最长 10 分钟的水合轮询。
func TestHydrateFlightsDedup(t *testing.T) {
	flights := newHydrateFlights()
	var calls atomic.Int64
	const workers = 16

	var wg sync.WaitGroup
	errs := make([]error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = flights.do("same-file", func() error {
				calls.Add(1)
				time.Sleep(50 * time.Millisecond) // 放大并发窗口
				return nil
			})
		}(i)
	}
	wg.Wait()

	if got := calls.Load(); got != 1 {
		t.Fatalf("fn 应只执行一次，实际 %d 次", got)
	}
	for i, err := range errs {
		if err != nil {
			t.Fatalf("worker %d: %v", i, err)
		}
	}

	// 首次执行失败：所有等待者共享错误，且下一次调用重新执行（不缓存失败）。
	var calls2 atomic.Int64
	wantErr := errors.New("hydrate failed")
	var wg2 sync.WaitGroup
	errs2 := make([]error, workers)
	for i := 0; i < workers; i++ {
		wg2.Add(1)
		go func(idx int) {
			defer wg2.Done()
			errs2[idx] = flights.do("bad-file", func() error {
				calls2.Add(1)
				return wantErr
			})
		}(i)
	}
	wg2.Wait()
	for i, err := range errs2 {
		if !errors.Is(err, wantErr) {
			t.Fatalf("worker %d 应拿到共享错误，实际 %v", i, err)
		}
	}
	if err := flights.do("bad-file", func() error { return nil }); err != nil {
		t.Fatalf("失败后的新调用应重新执行而不是缓存错误: %v", err)
	}
}

// 大小校验：syncthing 落盘是临时文件+原子重命名，但水合回调的等待实现
// 可能提前返回——按对端索引大小再核一遍，防读端拿到不完整内容。
func TestWaitFileReady(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "f.bin")

	// 大小未知（wantSize<0）：存在即可
	if err := os.WriteFile(real, []byte("xx"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := waitFileReady(real, -1, time.Second); err != nil {
		t.Fatalf("未知大小应只看存在性: %v", err)
	}

	// 大小一致：立即通过
	if err := waitFileReady(real, 2, time.Second); err != nil {
		t.Fatalf("大小一致应立即通过: %v", err)
	}

	// 尚不存在 → 期间落盘 → 通过（异步写模拟同步引擎拉取）
	late := filepath.Join(dir, "late.bin")
	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = os.WriteFile(late, []byte("12345"), 0o644)
	}()
	if err := waitFileReady(late, 5, 3*time.Second); err != nil {
		t.Fatalf("异步落盘应等到: %v", err)
	}

	// 大小一直不一致 → 超时报错
	if err := waitFileReady(real, 999, 400*time.Millisecond); err == nil {
		t.Fatal("大小不符应超时失败")
	} else if want := "预期大小"; !contains(err.Error(), want) {
		t.Fatalf("错误信息应包含 %q: %v", want, err)
	}

	// 一直不存在 → 超时报错
	if err := waitFileReady(filepath.Join(dir, "nope.bin"), -1, 300*time.Millisecond); err == nil {
		t.Fatal("不存在应超时失败")
	}
}

// Lister 查预期大小：命中返回条目大小；目录/不存在/查询失败返回 -1。
func TestEntrySizeFromLister(t *testing.T) {
	l := &memListerPure{entries: map[string][]Entry{
		"":     {{Name: "a.txt", Size: 42}, {Name: "docs", IsDir: true}},
		"docs": {{Name: "b.bin", Size: 7}},
	}}
	if got := entrySizeFromLister(l, "a.txt"); got != 42 {
		t.Fatalf("a.txt 大小: %d", got)
	}
	if got := entrySizeFromLister(l, "docs/b.bin"); got != 7 {
		t.Fatalf("嵌套路径大小: %d", got)
	}
	if got := entrySizeFromLister(l, "docs"); got != -1 {
		t.Fatalf("目录应返回 -1: %d", got)
	}
	if got := entrySizeFromLister(l, "missing"); got != -1 {
		t.Fatalf("不存在应返回 -1: %d", got)
	}
	if got := entrySizeFromLister(errLister{}, "a.txt"); got != -1 {
		t.Fatalf("Lister 出错应返回 -1: %d", got)
	}
}

// memListerPure 与 fs_test.go 的 memLister 等价（fs_test 在 cgofuse tag 下，
// 这里的纯版保证水合测试在全平台可跑）。
type memListerPure struct {
	entries map[string][]Entry
}

func (m *memListerPure) List(relDir string) ([]Entry, error) {
	return m.entries[relDir], nil
}

type errLister struct{}

func (errLister) List(string) ([]Entry, error) { return nil, errors.New("boom") }

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
