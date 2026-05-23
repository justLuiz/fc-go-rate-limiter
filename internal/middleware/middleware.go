package middleware

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/fullcycle/rate-limiter/internal/limiter"
)

type Config struct {
	IPMaxRequests    int64
	TokenMaxRequests int64
	WindowDuration   time.Duration
}

func RateLimiter(strategy limiter.Strategy, config Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
			apiKeyHeader := request.Header.Get("API_KEY")

			var rateLimitKey string
			var requestLimit int64

			if apiKeyHeader != "" {
				rateLimitKey = "token:" + apiKeyHeader
				requestLimit = config.TokenMaxRequests
			} else {
				clientIP, _, _ := net.SplitHostPort(request.RemoteAddr)
				if clientIP == "" {
					clientIP = request.RemoteAddr
				}
				rateLimitKey = "ip:" + clientIP
				requestLimit = config.IPMaxRequests
			}

			result, err := strategy.Allow(request.Context(), rateLimitKey, requestLimit, config.WindowDuration)
			if err != nil {
				fmt.Printf("rate limiter error: %v\n", err)
				next.ServeHTTP(responseWriter, request)
				return
			}

			if !result.Allowed {
				responseWriter.Header().Set("Content-Type", "text/plain")
				responseWriter.Header().Set("Retry-After", fmt.Sprintf("%d", int64(result.RetryAfter.Seconds())))
				responseWriter.WriteHeader(http.StatusTooManyRequests)
				responseWriter.Write([]byte("you have reached the maximum number of requests or actions allowed within a certain time frame"))
				return
			}

			next.ServeHTTP(responseWriter, request)
		})
	}
}
