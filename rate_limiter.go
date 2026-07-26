package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)


// TokenBucket 令牌桶限流器（基于 Redis）
type TokenBucket struct {
	key        string        // Redis key
	capacity   int           // 桶容量（最大令牌数）
	refillRate float64       // 每秒补充的令牌数
	rdb        *redis.Client // Redis 客户端
	mu         sync.Mutex    // 保护本地缓存
	lastUpdate time.Time     // 上次更新时间
	tokens     float64       // 本地缓存的令牌数（用于快速检查）
}

// NewTokenBucket 创建令牌桶限流器
func NewTokenBucket(rdb *redis.Client, key string, capacity int, refillRate float64) *TokenBucket {
	return &TokenBucket{
		key:        key,
		capacity:   capacity,
		refillRate: refillRate,
		rdb:        rdb,
		tokens:     float64(capacity), // 初始时桶满
		lastUpdate: time.Now(),
	}
}

// Allow 检查是否允许请求（非阻塞）
func (tb *TokenBucket) Allow(ctx context.Context) (bool, error) {
	// 分散验证：在限流检查时验证授权

	if tb.capacity <= 0 {
		return true, nil // 容量为0表示不限流
	}

	now := time.Now()
	tb.mu.Lock()
	defer tb.mu.Unlock()

	// 计算应该补充的令牌数
	elapsed := now.Sub(tb.lastUpdate).Seconds()
	tokensToAdd := elapsed * tb.refillRate

	// 更新本地缓存
	tb.tokens = tb.tokens + tokensToAdd
	if tb.tokens > float64(tb.capacity) {
		tb.tokens = float64(tb.capacity)
	}
	tb.lastUpdate = now

	// 快速检查：如果本地缓存有足够令牌，直接允许（减少 Redis 访问）
	if tb.tokens >= 1.0 {
		tb.tokens -= 1.0
		return true, nil
	}

	// 本地缓存不足：用 Redis 哈希 + WATCH/MULTI/EXEC 做与原先 Lua 等价的原子扣减
	nowMs := now.UnixMilli()
	allowed, err := tb.allowViaRedisWatch(ctx, nowMs)
	if err != nil {
		return true, fmt.Errorf("限流检查失败，允许请求: %w", err)
	}
	if allowed {
		tb.tokens -= 1.0
		if tb.tokens < 0 {
			tb.tokens = 0
		}
	}
	return allowed, nil
}

// allowViaRedisWatch 在 Redis 上原子地补充令牌并尝试消费 1（与历史 Lua 逻辑一致）
func (tb *TokenBucket) allowViaRedisWatch(ctx context.Context, nowMs int64) (bool, error) {
	const maxRetries = 32
	capF := float64(tb.capacity)
	for i := 0; i < maxRetries; i++ {
		var allowed bool
		err := tb.rdb.Watch(ctx, func(tx *redis.Tx) error {
			m, e := tx.HGetAll(ctx, tb.key).Result()
			if e != nil {
				return e
			}
			tokens := capF
			if s, ok := m["tokens"]; ok && s != "" {
				if v, err := strconv.ParseFloat(s, 64); err == nil {
					tokens = v
				}
			}
			lastUpdate := nowMs
			if s, ok := m["lastUpdate"]; ok && s != "" {
				if v, err := strconv.ParseInt(s, 10, 64); err == nil {
					lastUpdate = v
				}
			}
			elapsed := float64(nowMs-lastUpdate) / 1000.0
			tokens += elapsed * tb.refillRate
			if tokens > capF {
				tokens = capF
			}
			allowed = tokens >= 1.0
			if allowed {
				tokens -= 1.0
			}
			_, e = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.HSet(ctx, tb.key,
					"tokens", formatTokenBucketFloat(tokens),
					"lastUpdate", strconv.FormatInt(nowMs, 10),
				)
				pipe.Expire(ctx, tb.key, time.Hour)
				return nil
			})
			return e
		}, tb.key)
		if err == nil {
			return allowed, nil
		}
		if errors.Is(err, redis.TxFailedErr) {
			continue
		}
		return false, err
	}
	return false, fmt.Errorf("限流事务冲突过多")
}

func formatTokenBucketFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', 8, 64)
}

// Wait 等待直到允许请求（阻塞）
func (tb *TokenBucket) Wait(ctx context.Context) error {
	for {
		allowed, err := tb.Allow(ctx)
		if err != nil {
			return err
		}
		if allowed {
			return nil
		}

		// 计算需要等待的时间
		tb.mu.Lock()
		needed := 1.0 - tb.tokens
		waitTime := time.Duration(needed/tb.refillRate*1000) * time.Millisecond
		if waitTime > 1*time.Second {
			waitTime = 1 * time.Second // 最多等待1秒
		}
		tb.mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
			// 继续等待
		}
	}
}

// getRateLimiterForHID 获取指定 HID 的限流器（如果启用）
func getRateLimiterForHID(hid int) *TokenBucket {
	if !rateLimitEnabled {
		return nil
	}
	return ensureRateLimiterForHID(hid)
}

func ensureRateLimiterForHID(hid int) *TokenBucket {
	if !rateLimitEnabled || rdb == nil {
		return nil
	}
	rl := getEffectiveRateLimitForHID(hid)
	if !rl.Enabled || rl.PerHIDMaxPerSecond <= 0 {
		return nil
	}
	rateLimitMu.RLock()
	limiter, exists := perHIDRateLimiters[hid]
	rateLimitMu.RUnlock()
	if exists && limiter != nil && limiter.capacity == rl.PerHIDMaxPerSecond {
		return limiter
	}
	limiter = NewTokenBucket(rdb, fmt.Sprintf("rate_limit:hid:%d", hid), rl.PerHIDMaxPerSecond, float64(rl.PerHIDMaxPerSecond))
	rateLimitMu.Lock()
	perHIDRateLimiters[hid] = limiter
	rateLimitMu.Unlock()
	return limiter
}

// checkRateLimit 检查限流（全局和按 HID）
func checkRateLimit(ctx context.Context, hid int) (bool, error) {
	rl := getEffectiveRateLimitForHID(hid)
	if !rl.Enabled {
		return true, nil
	}

	// 检查全局限流
	if rl.GlobalMaxPerSecond > 0 {
		if globalRateLimiter == nil || globalRateLimiter.capacity != rl.GlobalMaxPerSecond {
			globalRateLimiter = NewTokenBucket(rdb, "rate_limit:global", rl.GlobalMaxPerSecond, float64(rl.GlobalMaxPerSecond))
		}
	}
	if globalRateLimiter != nil {
		allowed, err := globalRateLimiter.Allow(ctx)
		if err != nil {
			// 限流检查失败，为了可用性，允许请求
			return true, err
		}
		if !allowed {
			return false, fmt.Errorf("全局限流：已达到每秒最大请求数限制")
		}
	}

	// 检查按 HID 限流
	hidLimiter := getRateLimiterForHID(hid)
	if hidLimiter != nil {
		allowed, err := hidLimiter.Allow(ctx)
		if err != nil {
			// 限流检查失败，为了可用性，允许请求
			return true, err
		}
		if !allowed {
			// 获取货源名称
			name := getHuoyuanName(hid)
			if name == "" {
				name = fmt.Sprintf("hid%d", hid)
			}
			return false, fmt.Errorf("%s 限流：已达到每秒最大请求数限制", name)
		}
	}

	return true, nil
}
