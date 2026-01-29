package health_checker

import (
	"context"
	"mooc-manus/internal/infra/storage"
	"time"
)

type RedisHealthChecker struct {
}

func (r *RedisHealthChecker) Check() HealthStatus {
	status := HealthStatus{}
	status.Service = "redis"
	redisCli := storage.GetRedisClient()
	timeoutCtx, cancelFunc := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelFunc()

	if err := redisCli.Ping(timeoutCtx).Err(); err != nil {
		status.Status = UnHealthyStatus
		status.Detail = err.Error()
	} else {
		status.Status = HealthyStatus
	}
	return status
}
