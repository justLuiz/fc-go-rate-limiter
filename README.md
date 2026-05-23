# Rate Limiter

An HTTP rate limiter middleware backed by Redis, using the Strategy pattern.

## Configuration

Edit the `.env` file to configure the rate limiter:

| Variable | Description | Default |
|---|---|---|
| `REDIS_ADDR` | Redis server address | `redis:6379` |
| `REDIS_PASSWORD` | Redis password | (empty) |
| `REDIS_DB` | Redis database number | `0` |
| `IP_MAX_REQUESTS` | Max requests per IP per window | `10` |
| `TOKEN_MAX_REQUESTS` | Max requests per API token per window | `100` |
| `WINDOW_SECONDS` | Time window in seconds | `1` |
| `BLOCK_DURATION_SECONDS` | Duration to block after limit exceeded | `300` |

## How to Run

```bash
docker-compose up --build
```

## How to Test

Test IP-based rate limiting (send 15 requests, expect 429 after limit):

```bash
for i in {1..15}; do curl -s -o /dev/null -w "%{http_code}\n" localhost:8080/; done
```

Test with API token (uses TOKEN_MAX_REQUESTS limit):

```bash
curl -H "API_KEY: mytoken" localhost:8080/
```

## Strategy Pattern

The rate limiter uses the Strategy pattern defined in `internal/limiter/limiter.go`:

```go
type Strategy interface {
    Allow(ctx context.Context, key string, limit int64, windowDuration time.Duration) (Result, error)
    IsBlocked(ctx context.Context, key string) (bool, time.Duration, error)
    Block(ctx context.Context, key string, blockDuration time.Duration) error
}
```

The `RedisStrategy` in `internal/limiter/redis_strategy.go` implements this interface using Redis as the backend. To swap in a different backend (e.g., in-memory, Memcached, or a database), implement the `Strategy` interface and pass the new implementation to `middleware.RateLimiter(yourStrategy)` in `main.go`. No other code needs to change.
