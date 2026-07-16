package mq

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newMiniRedis 起 miniredis 并返回 go-redis 客户端；t.Cleanup 里回收资源。
func newMiniRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(func() { mr.Close() })
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return mr, rdb
}

// TestCaseTokenGate_Concurrency 8 goroutine 抢桶大小=4，恰好 4 成功
func TestCaseTokenGate_Concurrency(t *testing.T) {
	_, rdb := newMiniRedis(t)
	gate := NewCaseTokenGate(rdb, 4, 10*time.Minute)

	var won int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := gate.Acquire(context.Background(), "case-1")
			require.NoError(t, err)
			if ok {
				atomic.AddInt32(&won, 1)
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, int32(4), atomic.LoadInt32(&won))
}

// TestCaseTokenGate_Release 释放后桶位可再次获取
func TestCaseTokenGate_Release(t *testing.T) {
	_, rdb := newMiniRedis(t)
	gate := NewCaseTokenGate(rdb, 1, time.Minute)
	ctx := context.Background()

	ok1, err := gate.Acquire(ctx, "case-2")
	require.NoError(t, err)
	require.True(t, ok1)

	ok2, err := gate.Acquire(ctx, "case-2")
	require.NoError(t, err)
	require.False(t, ok2, "桶大小=1 时第二次 Acquire 必须失败")

	require.NoError(t, gate.Release(ctx, "case-2"))

	ok3, err := gate.Acquire(ctx, "case-2")
	require.NoError(t, err)
	require.True(t, ok3, "Release 后应能再次 Acquire")
}

// TestCaseTokenGate_TTLExpire TTL 到期后计数自愈
func TestCaseTokenGate_TTLExpire(t *testing.T) {
	mr, rdb := newMiniRedis(t)
	gate := NewCaseTokenGate(rdb, 1, 2*time.Second)
	ctx := context.Background()

	ok, err := gate.Acquire(ctx, "case-3")
	require.NoError(t, err)
	require.True(t, ok)

	// FastForward 让 miniredis 时钟前进，模拟 TTL 过期
	mr.FastForward(3 * time.Second)

	ok2, err := gate.Acquire(ctx, "case-3")
	require.NoError(t, err)
	require.True(t, ok2, "TTL 过期后应能再次 Acquire")
}

// TestCaseTokenGate_Defaults 传入非法参数走默认值
func TestCaseTokenGate_Defaults(t *testing.T) {
	_, rdb := newMiniRedis(t)
	gate := NewCaseTokenGate(rdb, 0, 0) // 走 limit=4/ttl=20m 默认
	ctx := context.Background()

	// 默认 limit=4：第 5 次开始失败
	var won int
	for i := 0; i < 6; i++ {
		ok, err := gate.Acquire(ctx, "case-default")
		require.NoError(t, err)
		if ok {
			won++
		}
	}
	assert.Equal(t, 4, won)
}
