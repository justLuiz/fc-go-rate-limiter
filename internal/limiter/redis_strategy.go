package limiter

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisStrategy struct {
	redisClient   *redis.Client
	blockDuration time.Duration
}

func NewRedisStrategy(redisClient *redis.Client, blockDuration time.Duration) *RedisStrategy {
	return &RedisStrategy{redisClient: redisClient, blockDuration: blockDuration}
}

func (redisStrategy *RedisStrategy) IsBlocked(ctx context.Context, key string) (bool, time.Duration, error) {
	blockKey := "block:" + key
	timeToLive, err := redisStrategy.redisClient.TTL(ctx, blockKey).Result()
	if err != nil {
		return false, 0, err
	}
	if timeToLive > 0 {
		return true, timeToLive, nil
	}
	return false, 0, nil
}

func (redisStrategy *RedisStrategy) Block(ctx context.Context, key string, blockDuration time.Duration) error {
	blockKey := "block:" + key
	blockDurationSeconds := int64(blockDuration.Seconds())
	return redisStrategy.redisClient.Set(ctx, blockKey, "1", time.Duration(blockDurationSeconds)*time.Second).Err()
}

func (redisStrategy *RedisStrategy) Allow(ctx context.Context, key string, limit int64, windowDuration time.Duration) (Result, error) {
	isBlocked, timeToLive, err := redisStrategy.IsBlocked(ctx, key)
	if err != nil {
		return Result{}, err
	}
	if isBlocked {
		return Result{Allowed: false, RetryAfter: timeToLive}, nil
	}

	windowSeconds := int64(windowDuration.Seconds())
	if windowSeconds <= 0 {
		return Result{}, fmt.Errorf("window duration must be greater than zero")
	}
	bucketID := time.Now().Unix() / windowSeconds
	rateLimitKey := fmt.Sprintf("rl:%s:%d", key, bucketID)

	luaScript := redis.NewScript(`
local count = redis.call("INCR", KEYS[1])
if count == 1 then
    redis.call("EXPIRE", KEYS[1], ARGV[1])
end
local ttl = redis.call("TTL", KEYS[1])
return {count, ttl}
`)

	scriptResult, err := luaScript.Run(ctx, redisStrategy.redisClient, []string{rateLimitKey}, windowSeconds).Result()
	if err != nil {
		return Result{}, err
	}

	resultValues := scriptResult.([]interface{})
	currentCount := resultValues[0].(int64)

	if currentCount > limit {
		blockErr := redisStrategy.Block(ctx, key, redisStrategy.blockDuration)
		if blockErr != nil {
			return Result{}, blockErr
		}
		return Result{Allowed: false, RetryAfter: redisStrategy.blockDuration}, nil
	}

	return Result{Allowed: true, Remaining: limit - currentCount}, nil
}
