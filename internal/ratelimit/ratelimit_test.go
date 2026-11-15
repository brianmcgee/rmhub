package ratelimit

import (
	"strconv"
	"sync"
	"testing"
	"time"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newLimiter(n int) (*Limiter, *clock) {
	c := &clock{t: time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)}
	l := New(n)
	l.SetClock(c.now)
	return l, c
}

func TestBurstAndRefill(t *testing.T) {
	l, c := newLimiter(10)
	for i := range 10 {
		if ok, _ := l.Allow("a"); !ok {
			t.Fatalf("request %d was refused", i+1)
		}
	}
	ok, wait := l.Allow("a")
	if ok {
		t.Fatal("request 11 was accepted")
	}
	if wait != 6*time.Second {
		t.Errorf("wait = %v, want 6s", wait)
	}
	if ok, _ := l.Allow("b"); !ok {
		t.Error("a second key was refused")
	}
	c.t = c.t.Add(6 * time.Second)
	if ok, _ := l.Allow("a"); !ok {
		t.Error("the request after 6 seconds was refused")
	}
	if ok, _ := l.Allow("a"); ok {
		t.Error("a second request after 6 seconds was accepted")
	}
}

func TestBucketDoesNotGrowAboveLimit(t *testing.T) {
	l, c := newLimiter(2)
	l.Allow("a")
	c.t = c.t.Add(time.Hour)
	for i := range 2 {
		if ok, _ := l.Allow("a"); !ok {
			t.Fatalf("request %d was refused", i+1)
		}
	}
	if ok, _ := l.Allow("a"); ok {
		t.Error("request 3 was accepted")
	}
}

func TestOff(t *testing.T) {
	l := New(-1)
	for range 1000 {
		if ok, _ := l.Allow("a"); !ok {
			t.Fatal("a limiter that is off refused a request")
		}
	}
}

func TestSweep(t *testing.T) {
	l, c := newLimiter(10)
	for i := range 50 {
		l.Allow(strconv.Itoa(i))
	}
	if got := l.Len(); got != 50 {
		t.Fatalf("Len = %d, want 50", got)
	}
	// After 6 seconds each bucket is full again. The sweep runs at most
	// one time in each minute.
	c.t = c.t.Add(time.Minute)
	l.Allow("other")
	if got := l.Len(); got != 1 {
		t.Errorf("Len after the sweep = %d, want 1", got)
	}
}

func TestKeyBound(t *testing.T) {
	l, c := newLimiter(10)
	for i := range MaxKeys {
		if ok, _ := l.Allow(strconv.Itoa(i)); !ok {
			t.Fatalf("key %d was refused", i+1)
		}
	}
	ok, wait := l.Allow("one-more")
	if ok {
		t.Error("key 100 001 was accepted")
	}
	if wait < time.Second {
		t.Errorf("wait = %v, want 1s or more", wait)
	}
	if ok, _ := l.Allow("0"); !ok {
		t.Error("a known key was refused at the bound")
	}
	// The buckets fill, so the bound must not stay.
	c.t = c.t.Add(10 * time.Second)
	if ok, _ := l.Allow("one-more"); !ok {
		t.Error("a new key was refused after the buckets were full again")
	}
}

func TestParallel(t *testing.T) {
	l := New(1000)
	var wg sync.WaitGroup
	var mu sync.Mutex
	accepted := 0
	for range 8 {
		wg.Go(func() {
			for range 500 {
				if ok, _ := l.Allow("a"); ok {
					mu.Lock()
					accepted++
					mu.Unlock()
				}
			}
		})
	}
	wg.Wait()
	// The bucket can fill a little while the test runs.
	if accepted < 1000 || accepted > 1100 {
		t.Errorf("accepted = %d, want 1000 to 1100", accepted)
	}
}
