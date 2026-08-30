package placeholder

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// 水合辅助逻辑（平台无关，不依赖 FUSE 头文件）：并发去重 + 大小校验。
// 虚拟层（fs.go，cgofuse 构建）与未来其他水合入口共用。

// splitVirt 把虚拟层相对路径按 "/" 切成 (目录, 文件名)。
// 虚拟路径统一用 "/" 分隔（macFUSE/FUSE 与 WinFsp-FUSE 均如此），
// 不能依赖 filepath.Split（Windows 上按 "\" 切，会切错嵌套路径）。
func splitVirt(p string) (dir, base string) {
	p = strings.Trim(p, "/")
	if p == "" {
		return "", ""
	}
	i := strings.LastIndex(p, "/")
	if i < 0 {
		return "", p
	}
	return p[:i], p[i+1:]
}

// hydrateFlights 对同一文件的去重水合：文件管理器会为缩略图/预览并发打开
// 多个句柄，不去重会为同一文件起多个 Hydrate 轮询循环（每个最长 10 分钟）。
type hydrateFlights struct {
	mu       sync.Mutex
	inFlight map[string]*hydrateCall
}

type hydrateCall struct {
	wg  sync.WaitGroup
	err error
}

func newHydrateFlights() *hydrateFlights {
	return &hydrateFlights{inFlight: make(map[string]*hydrateCall)}
}

// do 对同一 key（通常为解析后的本地真实路径）的并发调用只执行一次 fn，
// 其余等待首次执行完成并共享其结果。
func (h *hydrateFlights) do(key string, fn func() error) error {
	h.mu.Lock()
	if c, ok := h.inFlight[key]; ok {
		h.mu.Unlock()
		c.wg.Wait()
		return c.err
	}
	c := &hydrateCall{}
	c.wg.Add(1)
	h.inFlight[key] = c
	h.mu.Unlock()

	c.err = fn()

	h.mu.Lock()
	delete(h.inFlight, key)
	h.mu.Unlock()
	c.wg.Done()
	return c.err
}

// waitFileReady 等待本地文件存在且大小与对端索引一致。
//
// syncthing 经临时文件落盘后原子重命名——路径出现即完整；但水合回调的
// 等待逻辑可能提前返回（自定义实现/竞态），这里按 wantSize 再核一遍，
// 不匹配（或尚不存在）时轮询兜底，避免读端拿到不完整内容。wantSize < 0
// 表示大小未知，存在即通过。
func waitFileReady(realPath string, wantSize int64, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if fi, err := os.Stat(realPath); err == nil {
			if wantSize < 0 || fi.Size() == wantSize {
				return nil
			}
		}
		if time.Now().After(deadline) {
			if wantSize >= 0 {
				return fmt.Errorf("等待 %s 达到预期大小 %d 超时（%s）", realPath, wantSize, timeout)
			}
			return fmt.Errorf("等待 %s 落盘超时（%s）", realPath, timeout)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// entrySizeFromLister 从 Lister 查某相对路径文件的预期大小；未知返回 -1。
func entrySizeFromLister(l Lister, relPath string) int64 {
	dir, base := splitVirt(relPath)
	entries, err := l.List(dir)
	if err != nil {
		return -1
	}
	for _, e := range entries {
		if e.Name == base && !e.IsDir {
			return e.Size
		}
	}
	return -1
}
