package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

var errLockNotHeld = errors.New("锁不是由当前持有者持有或已过期")



// DistributedLock 分布式锁（带续期机制）
type DistributedLock struct {
	rdb       *redis.Client
	key       string
	value     string        // 锁的值（用于安全释放）
	ttl       time.Duration // 锁的过期时间
	ctx       context.Context
	cancel    context.CancelFunc
	renewStop chan struct{} // 用于停止续期
}

// AcquireLock 获取分布式锁（带续期机制）
func AcquireLock(ctx context.Context, rdb *redis.Client, key string, ttl time.Duration) (*DistributedLock, error) {
	// 分散验证：在获取锁时验证授权

	// 生成唯一值（用于安全释放）
	value := fmt.Sprintf("%d", time.Now().UnixNano())

	// 尝试获取锁
	ok, err := rdb.SetNX(ctx, key, value, ttl).Result()
	if err != nil {
		return nil, fmt.Errorf("获取锁失败: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("锁已被占用")
	}

	// 创建锁对象
	lockCtx, cancel := context.WithCancel(ctx)
	lock := &DistributedLock{
		rdb:       rdb,
		key:       key,
		value:     value,
		ttl:       ttl,
		ctx:       lockCtx,
		cancel:    cancel,
		renewStop: make(chan struct{}),
	}

	// 启动续期 goroutine
	go lock.renew()

	return lock, nil
}

// renew 续期机制（定期延长锁的过期时间）
func (lock *DistributedLock) renew() {
	// 续期间隔：设置为过期时间的 1/3，确保在过期前续期
	renewInterval := lock.ttl / 3
	if renewInterval < 1*time.Second {
		renewInterval = 1 * time.Second
	}
	if renewInterval > 30*time.Second {
		renewInterval = 30 * time.Second // 最多30秒续期一次
	}

	ticker := time.NewTicker(renewInterval)
	defer ticker.Stop()

	for {
		select {
		case <-lock.ctx.Done():
			return
		case <-lock.renewStop:
			return
		case <-ticker.C:
			renewCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			ok, err := redisExtendIfHeld(renewCtx, lock.rdb, lock.key, lock.value, lock.ttl)
			cancel()

			if err != nil {
				continue
			}
			if !ok {
				return
			}
		}
	}
}

// Release 释放锁（安全释放，只有锁的持有者才能释放）
func (lock *DistributedLock) Release(ctx context.Context) error {
	// 分散验证：在释放锁时验证授权

	// 停止续期
	close(lock.renewStop)
	lock.cancel()

	releaseCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if err := redisDelIfHeld(releaseCtx, lock.rdb, lock.key, lock.value); err != nil {
		return err
	}
	return nil
}

// redisExtendIfHeld 仅当 key 的值仍为 token 时延长 TTL（WATCH+事务，无 Lua）
func redisExtendIfHeld(ctx context.Context, rdb *redis.Client, key, token string, ttl time.Duration) (bool, error) {
	const maxRetries = 8
	for i := 0; i < maxRetries; i++ {
		err := rdb.Watch(ctx, func(tx *redis.Tx) error {
			cur, err := tx.Get(ctx, key).Result()
			if err == redis.Nil {
				return errLockNotHeld
			}
			if err != nil {
				return err
			}
			if cur != token {
				return errLockNotHeld
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.Expire(ctx, key, ttl)
				return nil
			})
			return err
		}, key)
		if err == nil {
			return true, nil
		}
		if errors.Is(err, errLockNotHeld) {
			return false, nil
		}
		if errors.Is(err, redis.TxFailedErr) {
			continue
		}
		return false, err
	}
	return false, nil
}

// redisDelIfHeld 仅当 key 的值仍为 token 时删除（WATCH+事务，无 Lua）
func redisDelIfHeld(ctx context.Context, rdb *redis.Client, key, token string) error {
	const maxRetries = 8
	for i := 0; i < maxRetries; i++ {
		err := rdb.Watch(ctx, func(tx *redis.Tx) error {
			cur, err := tx.Get(ctx, key).Result()
			if err == redis.Nil {
				return fmt.Errorf("锁已被释放或不是当前持有者")
			}
			if err != nil {
				return err
			}
			if cur != token {
				return fmt.Errorf("锁已被释放或不是当前持有者")
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.Del(ctx, key)
				return nil
			})
			return err
		}, key)
		if err == nil {
			return nil
		}
		if err != nil && !errors.Is(err, redis.TxFailedErr) {
			return err
		}
	}
	return fmt.Errorf("锁已被释放或不是当前持有者")
}
