package bot

import (
	"sync"
	"time"
)

const (
	// 一般人速率限制
	rateLimitNormal    = 1 * time.Minute  // 前 20 次：每 1 分鐘 1 次
	rateLimitOverLimit = 15 * time.Minute // 超過 20 次後：每 15 分鐘 1 次
	rateLimitThreshold = 20               // 超過此次數後改用 overLimit 規則
)

type userRateState struct {
	lastRequestTime time.Time
	totalRequests   int
}

type rateLimiter struct {
	mu    sync.Mutex
	users map[int64]*userRateState
}

func newRateLimiter() *rateLimiter {
	rl := &rateLimiter{
		users: make(map[int64]*userRateState),
	}
	go rl.cleanupInactiveUsers()
	return rl
}

// cleanupInactiveUsers 定期清理長時間未使用的使用者記錄，防止記憶體無限增長。
func (r *rateLimiter) cleanupInactiveUsers() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		r.mu.Lock()
		cutoff := time.Now().Add(-24 * time.Hour)
		for uid, state := range r.users {
			if state.lastRequestTime.Before(cutoff) {
				delete(r.users, uid)
			}
		}
		r.mu.Unlock()
	}
}

// CheckRateLimit 檢查使用者是否可以發送請求。
// 回傳：
//   - allowed: 是否允許此次請求
//   - retryAfter: 若不允許，需等待的時間
//   - overLimit: 是否已超過 20 次（適用 15 分鐘限制）
func (r *rateLimiter) CheckRateLimit(userID int64) (allowed bool, retryAfter time.Duration, overLimit bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	state, exists := r.users[userID]
	if !exists {
		return true, 0, false
	}

	overLimit = state.totalRequests >= rateLimitThreshold
	limit := rateLimitNormal
	if overLimit {
		limit = rateLimitOverLimit
	}

	elapsed := now.Sub(state.lastRequestTime)
	if elapsed < limit {
		retryAfter = limit - elapsed
		return false, retryAfter, overLimit
	}

	return true, 0, overLimit
}

// RecordRequest 記錄使用者的一次請求
func (r *rateLimiter) RecordRequest(userID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	state, exists := r.users[userID]
	if !exists {
		r.users[userID] = &userRateState{
			lastRequestTime: now,
			totalRequests:   1,
		}
		return
	}

	state.lastRequestTime = now
	state.totalRequests++
}
