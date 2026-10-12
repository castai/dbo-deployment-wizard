package backend

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSyncCache(t *testing.T) {
	r := require.New(t)

	var c syncCache[string, int]

	_, ok := c.get("a")
	r.False(ok)

	c.put("a", 1)
	c.put("a", 2)

	got, ok := c.get("a")
	r.True(ok)
	r.Equal(2, got)
}

// Meaningful under -race: concurrent readers and writers must not trip it.
func TestSyncCache_ConcurrentUse(t *testing.T) {
	r := require.New(t)

	var c syncCache[int, int]

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Go(func() {
			c.put(i%5, i)
			_, _ = c.get(i % 5)
		})
	}
	wg.Wait()

	_, ok := c.get(0)
	r.True(ok)
}
