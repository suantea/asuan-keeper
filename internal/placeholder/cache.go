package placeholder

import (
	"sync"
	"time"
)

// browseCache 是占位符目录列表的小型 TTL 缓存。
//
// 背景：PlaceholderFS.Readdir 每次都经 ReleaseLister.List 转发到
// syncthing REST API（/rest/db/browse），浏览大目录时每个目录项都
// 往返一次 HTTP，延迟明显。这里按 relDir 缓存目录条目，TTL 到期后
// 自动失效（文件管理器滚动/重复 Readdir 时命中缓存，避免重复请求）。
//
// 并发安全：所有方法内部加锁；get 返回条目副本，调用方修改不影响缓存。
type browseCache struct {
	mu      sync.Mutex
	entries map[string]browseCacheEntry
	order   []string // 插入顺序（FIFO 淘汰用）
	ttl     time.Duration
	max     int
}

type browseCacheEntry struct {
	items    []Entry
	expireAt time.Time
}

// newBrowseCache 创建目录列表缓存。ttl<=0 视为永不主动过期（仅靠上限淘汰），
// max<=0 表示不限制条目数。
func newBrowseCache(ttl time.Duration, max int) *browseCache {
	return &browseCache{
		entries: make(map[string]browseCacheEntry),
		ttl:     ttl,
		max:     max,
	}
}

// get 返回缓存的目录条目；未命中或已过期返回 false。
// 返回的是副本，调用方可以安全持有。
func (c *browseCache) get(key string) ([]Entry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	e, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if c.ttl > 0 && time.Now().After(e.expireAt) {
		c.removeLocked(key)
		return nil, false
	}
	out := make([]Entry, len(e.items))
	copy(out, e.items)
	return out, true
}

// put 写入目录条目缓存；超过上限时按 FIFO 淘汰最旧的条目。
func (c *browseCache) put(key string, items []Entry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.entries[key]; !ok {
		c.order = append(c.order, key)
	}
	c.entries[key] = browseCacheEntry{
		items:    items,
		expireAt: time.Now().Add(c.ttl),
	}
	for c.max > 0 && len(c.entries) > c.max && len(c.order) > 0 {
		c.removeLocked(c.order[0])
	}
}

// removeLocked 删除指定 key 的缓存条目（调用方必须持锁）。
func (c *browseCache) removeLocked(key string) {
	delete(c.entries, key)
	for i, k := range c.order {
		if k == key {
			c.order = append(c.order[:i], c.order[i+1:]...)
			break
		}
	}
}
