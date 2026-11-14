package sync15

import (
	"container/list"
	"sync"

	"github.com/brianmcgee/rmhub/internal/hashtree"
)

// rootCache is an LRU cache of parsed root indexes. A blob never
// changes, so an entry never goes stale.
type rootCache struct {
	mu    sync.Mutex
	size  int
	order *list.List // front is the most recent
	items map[string]*list.Element
}

type cacheItem struct {
	key string
	ix  *hashtree.Index
}

func newRootCache(size int) *rootCache {
	return &rootCache{size: size, order: list.New(), items: map[string]*list.Element{}}
}

func (c *rootCache) get(key string) (*hashtree.Index, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(el)
	return el.Value.(*cacheItem).ix, true
}

func (c *rootCache) put(key string, ix *hashtree.Index) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.order.MoveToFront(el)
		return
	}
	c.items[key] = c.order.PushFront(&cacheItem{key: key, ix: ix})
	for c.order.Len() > c.size {
		last := c.order.Back()
		c.order.Remove(last)
		delete(c.items, last.Value.(*cacheItem).key)
	}
}
