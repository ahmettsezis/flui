package auth

import (
	"sync"
	"time"
)

// RateLimiter allows at most limit events per key within window (fixed window, in memory).
type RateLimiter struct {
	limit  int
	window time.Duration
	now    func() time.Time

	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	start time.Time
	count int
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{limit: limit, window: window, now: time.Now, buckets: map[string]*bucket{}}
}

func (r *RateLimiter) Allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	if len(r.buckets) > 10000 {
		for k, b := range r.buckets {
			if now.Sub(b.start) >= r.window {
				delete(r.buckets, k)
			}
		}
	}
	b, ok := r.buckets[key]
	if !ok || now.Sub(b.start) >= r.window {
		r.buckets[key] = &bucket{start: now, count: 1}
		return true
	}
	if b.count >= r.limit {
		return false
	}
	b.count++
	return true
}
