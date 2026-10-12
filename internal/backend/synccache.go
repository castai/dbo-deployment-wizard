package backend

import "sync"

// syncCache is a map safe for concurrent use; the zero value is ready.
type syncCache[K comparable, V any] struct {
	mu    sync.Mutex
	items map[K]V
}

func (c *syncCache[K, V]) get(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	value, ok := c.items[key]

	return value, ok
}

func (c *syncCache[K, V]) put(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.items == nil {
		c.items = map[K]V{}
	}
	c.items[key] = value
}
