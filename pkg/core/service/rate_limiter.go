package service

import (
	"fmt"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimiter implements a simple in-memory rate limiter
type RateLimiter struct {
	requests map[string][]time.Time
	mutex    sync.RWMutex
	limit    int
	window   time.Duration
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
}

// IsAllowed checks if a request is allowed for the given key
func (rl *RateLimiter) IsAllowed(key string) bool {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	now := time.Now()
	
	// Clean up old requests
	if requests, exists := rl.requests[key]; exists {
		var validRequests []time.Time
		for _, reqTime := range requests {
			if now.Sub(reqTime) < rl.window {
				validRequests = append(validRequests, reqTime)
			}
		}
		rl.requests[key] = validRequests
	}

	// Check if limit is exceeded
	if len(rl.requests[key]) >= rl.limit {
		return false
	}

	// Add current request
	rl.requests[key] = append(rl.requests[key], now)
	return true
}

// GetRemainingRequests returns the number of remaining requests for the key
func (rl *RateLimiter) GetRemainingRequests(key string) int {
	rl.mutex.RLock()
	defer rl.mutex.RUnlock()

	now := time.Now()
	count := 0
	
	if requests, exists := rl.requests[key]; exists {
		for _, reqTime := range requests {
			if now.Sub(reqTime) < rl.window {
				count++
			}
		}
	}

	remaining := rl.limit - count
	if remaining < 0 {
		return 0
	}
	return remaining
}

// EmailVerificationRateLimiter is a global rate limiter for email verification
var EmailVerificationRateLimiter = NewRateLimiter(3, 15*time.Minute) // 3 requests per 15 minutes

// CheckEmailVerificationRateLimit checks rate limit for email verification
func CheckEmailVerificationRateLimit(c *gin.Context, userID string) error {
	key := fmt.Sprintf("email_verification:%s", userID)
	
	if !EmailVerificationRateLimiter.IsAllowed(key) {
		remaining := EmailVerificationRateLimiter.GetRemainingRequests(key)
		return fmt.Errorf("rate limit exceeded. You can request %d more verification emails in 15 minutes", remaining)
	}
	
	return nil
}
