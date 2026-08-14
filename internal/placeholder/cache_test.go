package placeholder

import (
	"sync"
	"testing"
	"time"
)

func TestBrowseCacheHitAndMiss(t *testing.T) {
	c := newBrowseCache(time.Minute, 0)

	if _, ok := c.get("a"); ok {
		t.Fatal("未写入的 key 不应命中")
	}

	items := []Entry{{Name: "f1.txt", Size: 10}, {Name: "sub", IsDir: true}}
	c.put("a", items)

	got, ok := c.get("a")
	if !ok {
		t.Fatal("写入后应命中")
	}
	if len(got) != 2 || got[0].Name != "f1.txt" || !got[1].IsDir {
		t.Fatalf("缓存内容不符: %+v", got)
	}
}

func TestBrowseCacheExpire(t *testing.T) {
	c := newBrowseCache(50*time.Millisecond, 0)
	c.put("a", []Entry{{Name: "x"}})

	if _, ok := c.get("a"); !ok {
		t.Fatal("TTL 内应命中")
	}
	time.Sleep(80 * time.Millisecond)
	if _, ok := c.get("a"); ok {
		t.Fatal("TTL 过期后不应命中")
	}
}

func TestBrowseCacheMaxFIFO(t *testing.T) {
	c := newBrowseCache(time.Minute, 2)
	c.put("a", []Entry{{Name: "a"}})
	c.put("b", []Entry{{Name: "b"}})
	c.put("c", []Entry{{Name: "c"}})

	// 超出上限后最早写入的 a 应被淘汰
	if _, ok := c.get("a"); ok {
		t.Fatal("超出上限后最旧的 a 应被淘汰")
	}
	if _, ok := c.get("b"); !ok {
		t.Fatal("b 应仍在缓存")
	}
	if _, ok := c.get("c"); !ok {
		t.Fatal("c 应仍在缓存")
	}
}

func TestBrowseCachePutReplacesKeepsOrder(t *testing.T) {
	c := newBrowseCache(time.Minute, 2)
	c.put("a", []Entry{{Name: "a"}})
	c.put("b", []Entry{{Name: "b"}})
	// 覆盖已存在的 key 不应改变 FIFO 顺序，也不应重复计数
	c.put("a", []Entry{{Name: "a2"}})
	c.put("c", []Entry{{Name: "c"}})

	if _, ok := c.get("a"); ok {
		t.Fatal("a 被覆盖后仍按最早插入顺序被淘汰")
	}
	if _, ok := c.get("b"); !ok {
		t.Fatal("b 应仍在缓存")
	}
	if _, ok := c.get("c"); !ok {
		t.Fatal("c 应仍在缓存")
	}
}

func TestBrowseCacheGetReturnsCopy(t *testing.T) {
	c := newBrowseCache(time.Minute, 0)
	c.put("a", []Entry{{Name: "orig"}})

	got, ok := c.get("a")
	if !ok {
		t.Fatal("应命中")
	}
	// 修改返回的切片不应影响缓存内容
	got[0].Name = "mutated"
	got = append(got, Entry{Name: "extra"})

	again, _ := c.get("a")
	if len(again) != 1 || again[0].Name != "orig" {
		t.Fatalf("get 应返回副本，缓存被污染: %+v", again)
	}
}

func TestBrowseCacheConcurrent(t *testing.T) {
	c := newBrowseCache(time.Minute, 64)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				key := string(rune('a' + g))
				c.put(key, []Entry{{Name: "f", Size: int64(i)}})
				c.get(key)
			}
		}(g)
	}
	wg.Wait()
	if len(c.entries) > 64 {
		t.Fatalf("条目数超过上限: %d", len(c.entries))
	}
}
