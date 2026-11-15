// Package ratelimit limits the number of requests of a key in a time.
package ratelimit

import (
	"math"
	"strconv"
	"sync"
	"time"
)

// MaxKeys is the maximum number of buckets of one limiter. It bounds the
// memory when a client uses many keys.
const MaxKeys = 100_000

// sweepInterval is the minimum time between two sweeps.
const sweepInterval = time.Minute

// fullSweepInterval is the minimum time between two sweeps when the
// limiter has MaxKeys buckets.
const fullSweepInterval = time.Second

type bucket struct {
	units float64
	at    time.Time
}

// Limiter has one token bucket for each key. A bucket holds up to n
// units and fills at the rate of n units in one minute. A nil Limiter
// accepts each request.
type Limiter struct {
	n float64

	mu      sync.Mutex
	now     func() time.Time
	buckets map[string]bucket
	swept   time.Time
}

// New returns a limiter of n requests in one minute for each key. If n
// is not positive, New returns nil: the limit is off.
func New(n int) *Limiter {
	if n <= 0 {
		return nil
	}
	return &Limiter{n: float64(n), now: time.Now, buckets: map[string]bucket{}}
}

// RetryAfter returns the value of the Retry-After header for the wait
// time: the seconds as an integer, 1 or more.
func RetryAfter(wait time.Duration) string {
	return strconv.Itoa(max(1, int(math.Ceil(wait.Seconds()))))
}

// SetClock sets the clock. Tests use it.
func (l *Limiter) SetClock(now func() time.Time) { l.now = now }

// Len returns the number of buckets.
func (l *Limiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}

// units returns the units of the bucket at the time now.
func (l *Limiter) units(b bucket, now time.Time) float64 {
	return min(l.n, b.units+float64(now.Sub(b.at))*l.n/float64(time.Minute))
}

// sweep removes each bucket that is full. A full bucket is the same as
// no bucket.
func (l *Limiter) sweep(now time.Time) {
	for k, b := range l.buckets {
		if l.units(b, now) >= l.n {
			delete(l.buckets, k)
		}
	}
	l.swept = now
}

// Allow takes one unit from the bucket of the key. If the bucket is
// empty, Allow returns false and the time until one unit is available.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	if l == nil {
		return true, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if l.swept.IsZero() {
		l.swept = now
	}
	b, ok := l.buckets[key]
	full := !ok && len(l.buckets) >= MaxKeys
	if since := now.Sub(l.swept); since >= sweepInterval || full && since >= fullSweepInterval {
		l.sweep(now)
		full = !ok && len(l.buckets) >= MaxKeys
	}
	if full {
		// There is no space for a new bucket. Refuse the key as if its
		// bucket were empty.
		return false, time.Duration(float64(time.Minute) / l.n)
	}
	units := l.n
	if ok {
		units = l.units(b, now)
	}
	if units < 1 {
		return false, time.Duration((1 - units) * float64(time.Minute) / l.n)
	}
	l.buckets[key] = bucket{units: units - 1, at: now}
	return true, 0
}
