package mq

import (
	"time"

	"github.com/hibiken/asynq"

	"mooc-manus/config"
)

// StartServer 启动 asynq server，绑定 RunInstance handler。
// 调用方需在应用退出时对返回的 *asynq.Server 调用 Shutdown() 做优雅停机。
//
// 参数说明：
//   - cfg：asynq 使用的 Redis 连接（可与业务 redis 拆 DB）
//   - evalCfg：worker 并发度参数
//   - handler：TaskTypeRunInstance 处理器，见 RunInstanceHandler
//
// 队列配置：
//   - QueueHigh 权重 10、QueueDefault 权重 5，实现"重试优先"抢占策略；
//   - 总并发 = worker_concurrency_default + worker_concurrency_high；
//   - RetryDelayFunc 目前统一 30s，配合 handler 中 SkipRetry 精细控制。
func StartServer(cfg config.AsynqConfig, evalCfg config.EvaluationConfig, handler asynq.Handler) (*asynq.Server, error) {
	total := evalCfg.WorkerConcurrencyDefault + evalCfg.WorkerConcurrencyHigh
	if total <= 0 {
		total = 10
	}
	srv := asynq.NewServer(
		asynq.RedisClientOpt{
			Addr:     cfg.RedisAddr,
			Password: cfg.RedisPassword,
			DB:       cfg.RedisDB,
		},
		asynq.Config{
			Concurrency: total,
			Queues: map[string]int{
				QueueDefault: 5,
				QueueHigh:    10,
			},
			RetryDelayFunc: func(n int, e error, t *asynq.Task) time.Duration {
				// 与 spec §5.4 保持一致：统一 30s 短 backoff，
				// 配合 handler 内 SkipRetry / 直接返回 err 的策略路由。
				return 30 * time.Second
			},
		},
	)
	mux := asynq.NewServeMux()
	mux.Handle(TaskTypeRunInstance, handler)
	if err := srv.Start(mux); err != nil {
		return nil, err
	}
	return srv, nil
}
