package placeholder

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/suantea/asuan-keeper/internal/config"
)

// 自动释放策略的核心保证：
//   - 只动「超龄」文件，年龄窗口内的新文件绝不释放；
//   - 绝不触碰 syncthing 内部条目（.stfolder/.stversions/.stignore）；
//   - 已释放路径（规则命中）不再重复入计划；
//   - 最旧优先、补足水位缺口即停；
//   - 执行顺序契约：先写规则、再删本地实体、最后才 Scan（顺序错了
//     删除就会传播到对端）。
func TestPlanAutoRelease(t *testing.T) {
	dir := t.TempDir()
	oldTime := time.Now().Add(-30 * 24 * time.Hour)
	olderTime := time.Now().Add(-20 * 24 * time.Hour) // 比上者新、仍超龄
	newTime := time.Now().Add(-1 * time.Hour)

	write := func(rel, content string, mod time.Time) {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, mod, mod); err != nil {
			t.Fatal(err)
		}
	}
	// 引擎内部条目 + 一个 30 天前的旧大文件 + 一个 1 小时前的新文件 +
	// .stversions 里的超龄文件（绝不能动）。
	if err := os.MkdirAll(filepath.Join(dir, ".stfolder"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".stversions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".stignore"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	write("old-big.bin", "11111", oldTime)             // 5 字节，最旧
	write("nested/old-medium.bin", "2222", olderTime)  // 4 字节，次旧
	write("fresh.bin", "333", newTime)              // 新文件
	write(".stversions/old.bin", "44444", oldTime)  // 回收站内容，绝不动

	cfg := &config.Config{Folders: []config.Folder{{
		ID: "f1", Label: "media", Path: dir, Policy: config.PolicySync,
	}}}
	opts := AutoReleaseOptions{MinFreeBytes: 10, Age: 7 * 24 * time.Hour}
	// 磁盘只剩 3 字节 → 缺口 7 字节：最旧优先选中 old-big.bin（5B）后
	// 还差 2B → 再选 nested/old-medium.bin（4B）→ 补足，fresh.bin 不入选。
	free := func(string) (uint64, error) { return 3, nil }

	plans, results := planAutoRelease(cfg, opts, free)
	if len(results) != 1 || results[0].Err != nil || results[0].Skipped != "" {
		t.Fatalf("意外结果: %+v", results)
	}
	if len(plans) != 1 {
		t.Fatalf("应产出 1 个计划，实际 %d", len(plans))
	}
	p := plans[0]
	if len(p.relPaths) != 2 || p.relPaths[0] != "old-big.bin" || p.relPaths[1] != "nested/old-medium.bin" {
		t.Fatalf("计划应最旧优先且补足缺口即停: %v", p.relPaths)
	}
	if p.bytes != 9 {
		t.Fatalf("计划释放字节应为 9，实际 %d", p.bytes)
	}
	for _, rule := range p.rules {
		if rule != "(?d)/old-big.bin" && rule != "(?d)/nested/old-medium.bin" {
			t.Fatalf("规则形态异常: %q", rule)
		}
	}

	// 水位充足 → 跳过。
	plans2, results2 := planAutoRelease(cfg, opts, func(string) (uint64, error) { return 1000, nil })
	if len(plans2) != 0 || results2[0].Skipped == "" {
		t.Fatalf("水位充足应跳过: plans=%d %+v", len(plans2), results2)
	}
}

// 已在 .stignore 里的路径不再重复入选；文件夹级释放整体跳过。
func TestPlanAutoReleaseRespectsExistingRules(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-30 * 24 * time.Hour)
	for _, rel := range []string{"a.bin", "b.bin", "c.bin"} {
		p := filepath.Join(dir, rel)
		if err := os.WriteFile(p, []byte("xx"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, ".stignore"), []byte("(?d)/a.bin\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Folders: []config.Folder{{ID: "f1", Path: dir, Policy: config.PolicySync}}}
	plans, _ := planAutoRelease(cfg, AutoReleaseOptions{MinFreeBytes: 100, Age: 24 * time.Hour},
		func(string) (uint64, error) { return 0, nil })
	if len(plans) != 1 {
		t.Fatalf("应有 1 个计划")
	}
	for _, rel := range plans[0].relPaths {
		if rel == "a.bin" {
			t.Fatal("已释放路径不应重复入选")
		}
	}

	// 文件夹级 (?d)* → 整体跳过。
	if err := os.WriteFile(filepath.Join(dir, ".stignore"), []byte("(?d)*"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, results := planAutoRelease(cfg, AutoReleaseOptions{MinFreeBytes: 100, Age: 24 * time.Hour},
		func(string) (uint64, error) { return 0, nil })
	if results[0].Skipped == "" {
		t.Fatal("文件夹级已释放应跳过")
	}
}

// 执行契约：规则先落盘、再删实体、最后 Scan 一次。
func TestSweepExecuteOrder(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-30 * 24 * time.Hour)
	p := filepath.Join(dir, "old.bin")
	if err := os.WriteFile(p, []byte("xx"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Folders: []config.Folder{{ID: "f1", Path: dir, Policy: config.PolicySync}}}

	var events []string
	scans := 0
	scan := func(folderID string) error {
		// Scan 时规则必须已写入、本地实体必须已删除。
		rules, err := ReadStIgnore(dir)
		if err != nil || len(rules) == 0 {
			t.Errorf("Scan 时规则应已写入: %v %v", rules, err)
		}
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("Scan 时本地实体应已删除")
		}
		events = append(events, "scan")
		scans++
		return nil
	}

	results := Sweep(cfg, AutoReleaseOptions{MinFreeBytes: 1, Age: 24 * time.Hour},
		func(string) (uint64, error) { return 0, nil }, scan)
	if len(results) != 1 || results[0].Err != nil {
		t.Fatalf("Sweep 结果异常: %+v", results)
	}
	if scans != 1 || len(events) != 1 {
		t.Fatalf("应恰好 Scan 一次: %d", scans)
	}
	if _, err := os.Stat(filepath.Join(dir, ".stignore")); err != nil {
		t.Fatalf(".stignore 应存在: %v", err)
	}
}

// 引擎重扫失败要如实上抛，不能假装释放成功。
func TestSweepScanErrorSurfaces(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-30 * 24 * time.Hour)
	p := filepath.Join(dir, "old.bin")
	if err := os.WriteFile(p, []byte("xx"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Folders: []config.Folder{{ID: "f1", Path: dir, Policy: config.PolicySync}}}
	results := Sweep(cfg, AutoReleaseOptions{MinFreeBytes: 1, Age: 24 * time.Hour},
		func(string) (uint64, error) { return 0, nil },
		func(string) error { return errors.New("engine down") })
	if len(results) != 1 || results[0].Err == nil {
		t.Fatalf("Scan 失败应上抛: %+v", results)
	}
}

// releasedPathChecker 的规则形态。
func TestReleasedPathChecker(t *testing.T) {
	check := releasedPathChecker([]string{
		"(?d)/a.bin",
		"(?d)/dir",
		"(?d)/dir/",
		"(?d)/dir/**",
		"(?d)*",
		"*", // 非 (?d) 规则不参与
	})
	if !check("a.bin") {
		t.Fatal("精确规则应命中")
	}
	if !check("dir/nested/deep.bin") {
		t.Fatal("目录前缀应命中")
	}
	if check("other.bin") {
		t.Fatal("无关路径不应命中")
	}
}
