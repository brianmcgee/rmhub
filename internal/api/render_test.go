package api

import (
	"bytes"
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/brianmcgee/rmhub/internal/rendercache"
)

// TestRenderOnce checks that two parallel requests for one uncached image
// cause one render, and that a request waits for a free render slot.
func TestRenderOnce(t *testing.T) {
	cache, err := rendercache.New(t.TempDir(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	r := NewRenderer(cache, 1, slog.Default())
	// Take the only render slot, so that both requests wait in the
	// render of the key.
	r.sem <- struct{}{}
	var calls atomic.Int32
	do := func(context.Context) ([]byte, error) {
		calls.Add(1)
		return []byte("image"), nil
	}
	var wg sync.WaitGroup
	results := make([][]byte, 2)
	for i := range results {
		wg.Go(func() {
			b, err := r.image(context.Background(), "acct", "k", do)
			if err != nil {
				t.Error(err)
			}
			results[i] = b
		})
	}
	time.Sleep(100 * time.Millisecond)
	if calls.Load() != 0 {
		t.Fatal("a render started without a free slot")
	}
	<-r.sem
	wg.Wait()
	if calls.Load() != 1 || r.Renders.Load() != 1 {
		t.Fatalf("%d renders, want 1", calls.Load())
	}
	for _, b := range results {
		if !bytes.Equal(b, []byte("image")) {
			t.Fatalf("result %q", b)
		}
	}
	if b, ok := cache.Get("k"); !ok || string(b) != "image" {
		t.Fatal("the result is not in the cache")
	}
}
