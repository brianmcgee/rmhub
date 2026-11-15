// Package lru is a small LRU cache for values that never go stale, such
// as parsed blobs. It is safe for concurrent use.
package lru

import (
	"container/list"
	"sync"
)

// Cache keeps at most size values. It removes the least recently used
// value first.
type Cache[K comparable, V any] struct {
	mu    sync.Mutex
	size  int
	order *list.List // front is the most recent
	items map[K]*list.Element
}

type item[K comparable, V any] struct {
	key K
	val V
}

// New returns a cache for size values.
func New[K comparable, V any](size int) *Cache[K, V] {
	return &Cache[K, V]{size: size, order: list.New(), items: map[K]*list.Element{}}
}

// Get returns the value of the key.
func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		var zero V
		return zero, false
	}
	c.order.MoveToFront(el)
	return el.Value.(*item[K, V]).val, true
}

// Put adds the value. If the key exists, Put keeps the first value.
func (c *Cache[K, V]) Put(key K, val V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.order.MoveToFront(el)
		return
	}
	c.items[key] = c.order.PushFront(&item[K, V]{key: key, val: val})
	for c.order.Len() > c.size {
		last := c.order.Back()
		c.order.Remove(last)
		delete(c.items, last.Value.(*item[K, V]).key)
	}
}
