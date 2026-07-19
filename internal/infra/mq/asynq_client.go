package mq

import (
	"context"
	"errors"
	"time"

	"github.com/hibiken/asynq"

	"mooc-manus/config"
)

// Client Asynq 客户端封装，负责向队列投递评测任务。
// 使用 asynq 官方 RedisClientOpt，不复用项目通用 go-redis 客户端：
//  1. asynq 内部有连接池 + 状态机管理，直接复用外部 client 反而破坏抽象；
//  2. 官方文档示例统一使用 RedisClientOpt，兼容更稳定；
//  3. 若与业务 Redis 隔离到不同 DB，参数天然独立。
type Client struct {
	inner *asynq.Client
}

// NewClient 依据 config 构造 asynq 客户端；调用方需在 shutdown 时 Close。
func NewClient(cfg config.AsynqConfig) *Client {
	inner := asynq.NewClient(asynq.RedisClientOpt{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	return &Client{inner: inner}
}

// Close 释放底层连接池
func (c *Client) Close() error { return c.inner.Close() }

// EnqueueRunInstance 投递单个评测实例任务。
//
// 幂等策略：使用 asynq.Unique(24h) 基于 payload+queue 生成 uniqueness key；
// 当 24h 内同一 payload 再次投递，asynq 返回 ErrDuplicateTask，此处静默吞掉。
//
// 超时/重试策略：
//   - MaxRetry(0)：由 InstanceExecutor 内部通过状态机 + cron reconciler 掌控重试节奏，
//     asynq 层不做自动重试，避免与 CAS 语义打架；
//   - Timeout(20m)：单次消费的硬上限，与 spec §4.3 instance_total_timeout_sec=900s 留出余量；
//   - Retention(72h)：任务终态后保留 72h 便于观测。
func (c *Client) EnqueueRunInstance(ctx context.Context, instanceID string, attempt int) error {
	payload := RunInstancePayload{
		InstanceID: instanceID,
		Attempt:    attempt,
		EnqueuedAt: time.Now().Unix(),
	}
	body, err := payload.Marshal()
	if err != nil {
		return err
	}
	queue := QueueDefault
	_, err = c.inner.EnqueueContext(ctx,
		asynq.NewTask(TaskTypeRunInstance, body),
		asynq.Queue(queue),
		asynq.Unique(24*time.Hour),
		asynq.MaxRetry(0),
		asynq.Timeout(20*time.Minute),
		asynq.Retention(72*time.Hour),
	)
	if err != nil && errors.Is(err, asynq.ErrDuplicateTask) {
		// 24h 内重复入队被 unique 拦截：视为幂等成功
		return nil
	}
	return err
}
