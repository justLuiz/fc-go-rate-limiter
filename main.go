package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/fullcycle/rate-limiter/internal/limiter"
	"github.com/fullcycle/rate-limiter/internal/middleware"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

func main() {
	godotenv.Load()

	redisAddress := os.Getenv("REDIS_ADDR")
	redisPassword := os.Getenv("REDIS_PASSWORD")
	redisDatabaseNumber, _ := strconv.Atoi(os.Getenv("REDIS_DB"))
	blockDurationSeconds, _ := strconv.ParseInt(os.Getenv("BLOCK_DURATION_SECONDS"), 10, 64)
	blockDuration := time.Duration(blockDurationSeconds) * time.Second

	ipMaxRequests, _ := strconv.ParseInt(os.Getenv("IP_MAX_REQUESTS"), 10, 64)
	tokenMaxRequests, _ := strconv.ParseInt(os.Getenv("TOKEN_MAX_REQUESTS"), 10, 64)
	windowSeconds, _ := strconv.ParseInt(os.Getenv("WINDOW_SECONDS"), 10, 64)
	if windowSeconds <= 0 {
		windowSeconds = 1
	}
	windowDuration := time.Duration(windowSeconds) * time.Second

	redisClient := redis.NewClient(&redis.Options{
		Addr:     redisAddress,
		Password: redisPassword,
		DB:       redisDatabaseNumber,
	})

	redisRateLimitStrategy := limiter.NewRedisStrategy(redisClient, blockDuration)

	rateLimiterConfig := middleware.Config{
		IPMaxRequests:    ipMaxRequests,
		TokenMaxRequests: tokenMaxRequests,
		WindowDuration:   windowDuration,
	}

	helloWorldHandler := http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		responseWriter.WriteHeader(http.StatusOK)
		responseWriter.Write([]byte("Hello, World!"))
	})

	rateLimitedHandler := middleware.RateLimiter(redisRateLimitStrategy, rateLimiterConfig)(helloWorldHandler)

	http.Handle("/", rateLimitedHandler)

	fmt.Println("Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
