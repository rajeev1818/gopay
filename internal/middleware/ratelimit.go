package middleware

import (
	"net/http"
	"sync"
	"time"
)

type bucket struct {
	tokens    float64
	lastCheck time.Time
}

type RateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	rate     float64
	capacity float64
}

func NewRateLimiter(ratePerSecond, capacity float64) *RateLimiter {
	return &RateLimiter{
		buckets:  make(map[string]*bucket),
		rate:     ratePerSecond,
		capacity: capacity,
	}
}

func (r1 *RateLimiter) Allow(key string) bool {
	r1.mu.Lock()
	defer r1.mu.Unlock()

	now := time.Now()

	b, exists := r1.buckets[key]

	if !exists {
		r1.buckets[key] = &bucket{tokens: r1.capacity - 1, lastCheck: now}
		return true
	}

	elapsed := now.Sub(b.lastCheck).Seconds()
	b.tokens += elapsed * r1.rate
	if b.tokens > r1.capacity {
		b.tokens = r1.capacity
	}
	b.lastCheck = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true

}

func (rl *RateLimiter) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.RemoteAddr

			if !rl.Allow(key) {
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})

	}
}
