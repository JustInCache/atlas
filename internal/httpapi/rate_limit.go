package httpapi

import (
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Session-based rate limiter using token bucket algorithm
// Limits per user session (browser), not per IP address
// This allows multiple users from the same corporate IP/proxy
type rateLimiter struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	rate     rate.Limit
	burst    int
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rate:     rate.Limit(200.0 / 60.0), // 200 requests per minute per session
		burst:    50,                       // Allow burst of 50 requests (initial dashboard load)
	}
}

func (rl *rateLimiter) getLimiter(sessionID string) *rate.Limiter {
	rl.mu.RLock()
	limiter, exists := rl.limiters[sessionID]
	rl.mu.RUnlock()

	if exists {
		return limiter
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Double-check after acquiring write lock
	if limiter, exists := rl.limiters[sessionID]; exists {
		return limiter
	}

	limiter = rate.NewLimiter(rl.rate, rl.burst)
	rl.limiters[sessionID] = limiter

	// Start cleanup goroutine on first use
	if len(rl.limiters) == 1 {
		go rl.cleanup()
	}

	return limiter
}

// cleanup removes old limiters every 10 minutes to prevent memory leak
func (rl *rateLimiter) cleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		// Remove limiters that haven't been used recently (are at full capacity)
		for sessionID, limiter := range rl.limiters {
			if limiter.Tokens() == float64(rl.burst) {
				delete(rl.limiters, sessionID)
			}
		}
		rl.mu.Unlock()
	}
}

// rateLimitMiddleware enforces rate limiting per user session (browser)
// Uses atlas_session cookie set by sessionMiddleware
func rateLimitMiddleware() func(http.Handler) http.Handler {
	limiter := newRateLimiter()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get session ID from cookie (set by sessionMiddleware)
			sessionCookie, err := r.Cookie("atlas_session")
			if err != nil {
				// No session cookie - this shouldn't happen if sessionMiddleware runs first
				// Allow the request and let sessionMiddleware set the cookie
				next.ServeHTTP(w, r)
				return
			}

			sessionID := sessionCookie.Value

			// Get limiter for this session
			sessionLimiter := limiter.getLimiter(sessionID)

			// Check if request is allowed
			if !sessionLimiter.Allow() {
				http.Error(w, "Rate limit exceeded. Please slow down and try again.", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
