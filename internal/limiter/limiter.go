package limiter

import (
	"context"
	"time"
)

type Result struct {
	Allowed    bool
	Remaining  int64
	RetryAfter time.Duration
}

type Strategy interface {
	Allow(ctx context.Context, key string, limit int64, windowDuration time.Duration) (Result, error)
	IsBlocked(ctx context.Context, key string) (bool, time.Duration, error)
	Block(ctx context.Context, key string, blockDuration time.Duration) error
}
