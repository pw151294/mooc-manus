package storage

import (
	"context"
	"fmt"
	"mooc-manus/config"
	"time"

	"github.com/redis/go-redis/v9"
)

var redisCli Redis

func GetRedisClient() Redis {
	return redisCli
}

type Redis interface {
	Pipeline() redis.Pipeliner
	// string operator
	Del(ctx context.Context, keys ...string) *redis.IntCmd
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
	Incr(ctx context.Context, key string) *redis.IntCmd
	IncrBy(ctx context.Context, key string, value int64) *redis.IntCmd
	Exists(ctx context.Context, keys ...string) *redis.IntCmd
	SetEx(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
	Expire(ctx context.Context, key string, expiration time.Duration) *redis.BoolCmd
	ScanType(ctx context.Context, cursor uint64, match string, count int64, keyType string) *redis.ScanCmd

	// hset operator
	HGetAll(ctx context.Context, key string) *redis.MapStringStringCmd
	HGet(ctx context.Context, key, field string) *redis.StringCmd
	HSet(ctx context.Context, key string, values ...interface{}) *redis.IntCmd
	HSetEX(ctx context.Context, key string, fieldsAndValues ...string) *redis.IntCmd
	HExpire(ctx context.Context, key string, expiration time.Duration, fields ...string) *redis.IntSliceCmd
	HDel(ctx context.Context, key string, fields ...string) *redis.IntCmd
	HKeys(ctx context.Context, key string) *redis.StringSliceCmd
	HExists(ctx context.Context, key, field string) *redis.BoolCmd

	// list operator
	LPush(ctx context.Context, key string, values ...interface{}) *redis.IntCmd
	RPush(ctx context.Context, key string, values ...interface{}) *redis.IntCmd
	LPop(ctx context.Context, key string) *redis.StringCmd
	RPop(ctx context.Context, key string) *redis.StringCmd
	BRPop(ctx context.Context, timeout time.Duration, keys ...string) *redis.StringSliceCmd
	BLPop(ctx context.Context, timeout time.Duration, keys ...string) *redis.StringSliceCmd
	RPopCount(ctx context.Context, key string, count int) *redis.StringSliceCmd
	LLen(ctx context.Context, key string) *redis.IntCmd

	Close() error
	Ping(ctx context.Context) *redis.StatusCmd
	Publish(ctx context.Context, channel string, message interface{}) *redis.IntCmd
	Subscribe(ctx context.Context, channels ...string) *redis.PubSub
	Watch(ctx context.Context, fn func(*redis.Tx) error, keys ...string) error
	Scan(ctx context.Context, cursor uint64, match string, count int64) *redis.ScanCmd

	MGet(ctx context.Context, keys ...string) *redis.SliceCmd

	// set operator
	SAdd(ctx context.Context, key string, members ...interface{}) *redis.IntCmd
	SRem(ctx context.Context, key string, members ...interface{}) *redis.IntCmd
}

func InitRedis() error {
	// 根据模式初始化不同的redis工具类
	rCfg := config.Cfg.Redis
	opts := &redis.Options{
		Addr:         rCfg.Addr,
		Username:     rCfg.Username,
		Password:     rCfg.Password,
		DB:           rCfg.DB,
		PoolSize:     rCfg.PoolSize,
		MinIdleConns: rCfg.MinIdleConns,
	}
	redisCli = redis.NewClient(opts)
	if err := redisCli.Ping(context.Background()).Err(); err != nil {
		return fmt.Errorf("ping redis failed: %v", err)
	}
	return nil
}
