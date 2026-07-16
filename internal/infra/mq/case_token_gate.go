package mq

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// CaseTokenGate 单 case 级并发令牌桶
// 用途：同一 case 在整个集群范围内最多同时执行 N 个 instance（spec §4.6 case_concurrency_limit），
// 避免热点 case 打爆 verify_script 依赖或触发限流。
type CaseTokenGate interface {
	// Acquire 尝试为 caseID 抢一个令牌；true=成功，false=已满
	Acquire(ctx context.Context, caseID string) (bool, error)
	// Release 归还令牌（DECR）；调用者需确保 Acquire=true 后再 Release
	Release(ctx context.Context, caseID string) error
}

type caseTokenGateImpl struct {
	rdb   redis.UniversalClient
	limit int
	ttl   time.Duration
}

// NewCaseTokenGate 构造令牌桶实现。
//   - limit<=0 时默认 4（与 spec §4.6 case_concurrency_limit 默认值一致）
//   - ttl<=0 时默认 20 分钟（大于 instance_total_timeout_sec=900s，保障异常场景计数自愈）
func NewCaseTokenGate(rdb redis.UniversalClient, limit int, ttl time.Duration) CaseTokenGate {
	if limit <= 0 {
		limit = 4
	}
	if ttl <= 0 {
		ttl = 20 * time.Minute
	}
	return &caseTokenGateImpl{rdb: rdb, limit: limit, ttl: ttl}
}

// acquireScript 原子 INCR + 判上限：
//   - 首次 INCR=1 时设置 TTL，避免 crash 造成计数泄漏（TTL 内自愈）
//   - 溢出时 DECR 回退，返回 0
//   - 未溢出返回 1
var acquireScript = redis.NewScript(`
local n = tonumber(redis.call('INCR', KEYS[1]))
if n == 1 then redis.call('EXPIRE', KEYS[1], ARGV[2]) end
if n > tonumber(ARGV[1]) then
    redis.call('DECR', KEYS[1])
    return 0
end
return 1
`)

func (g *caseTokenGateImpl) Acquire(ctx context.Context, caseID string) (bool, error) {
	key := fmt.Sprintf("eval:concurrency:case:%s", caseID)
	ttlSec := int(g.ttl.Seconds())
	res, err := acquireScript.Run(ctx, g.rdb, []string{key}, g.limit, ttlSec).Result()
	if err != nil {
		return false, err
	}
	// go-redis 对 Lua 数字返回值统一为 int64
	if n, ok := res.(int64); ok {
		return n == 1, nil
	}
	return false, nil
}

func (g *caseTokenGateImpl) Release(ctx context.Context, caseID string) error {
	key := fmt.Sprintf("eval:concurrency:case:%s", caseID)
	return g.rdb.Decr(ctx, key).Err()
}
