package mq

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mooc-manus/config"
)

// TestClient_EnqueueRunInstance_Idempotent 验证 asynq.Unique 保证同 payload 重复入队幂等
func TestClient_EnqueueRunInstance_Idempotent(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	cfg := config.AsynqConfig{RedisAddr: mr.Addr()}
	c := NewClient(cfg)
	defer c.Close()

	ctx := context.Background()

	err = c.EnqueueRunInstance(ctx, "inst-1", 0, false)
	require.NoError(t, err)

	// 相同 payload 立即再入队应被 Unique 拦截，静默吞掉
	err = c.EnqueueRunInstance(ctx, "inst-1", 0, false)
	require.NoError(t, err)

	// 队列内只应有 1 条 pending
	ins := asynq.NewInspector(asynq.RedisClientOpt{Addr: mr.Addr()})
	defer ins.Close()

	info, err := ins.GetQueueInfo(QueueDefault)
	require.NoError(t, err)
	assert.Equal(t, 1, info.Pending, "重复投递应被 Unique 去重")
}

// TestClient_EnqueueRunInstance_HighQueue useHigh=true 走 QueueHigh
func TestClient_EnqueueRunInstance_HighQueue(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	cfg := config.AsynqConfig{RedisAddr: mr.Addr()}
	c := NewClient(cfg)
	defer c.Close()

	require.NoError(t, c.EnqueueRunInstance(context.Background(), "inst-h", 1, true))

	ins := asynq.NewInspector(asynq.RedisClientOpt{Addr: mr.Addr()})
	defer ins.Close()

	hi, err := ins.GetQueueInfo(QueueHigh)
	require.NoError(t, err)
	assert.Equal(t, 1, hi.Pending)

	// default 队列应为空
	def, err := ins.GetQueueInfo(QueueDefault)
	if err == nil {
		// 队列可能尚未创建；创建的话 Pending 应为 0
		assert.Equal(t, 0, def.Pending)
	}
}

// TestPayload_RoundTrip Marshal/Unmarshal 基本可用性
func TestPayload_RoundTrip(t *testing.T) {
	p := RunInstancePayload{InstanceID: "abc", Attempt: 2, EnqueuedAt: 12345}
	b, err := p.Marshal()
	require.NoError(t, err)

	var got RunInstancePayload
	require.NoError(t, got.Unmarshal(b))
	assert.Equal(t, p, got)
}
