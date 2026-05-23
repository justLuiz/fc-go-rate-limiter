package limiter_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fullcycle/rate-limiter/internal/limiter"
	"github.com/fullcycle/rate-limiter/internal/middleware"
)

type MockStrategy struct {
	counts      map[string]int64
	blockedKeys map[string]bool
}

func NewMockStrategy() *MockStrategy {
	return &MockStrategy{
		counts:      make(map[string]int64),
		blockedKeys: make(map[string]bool),
	}
}

func (mockStrategy *MockStrategy) IsBlocked(ctx context.Context, key string) (bool, time.Duration, error) {
	if mockStrategy.blockedKeys[key] {
		return true, 5 * time.Minute, nil
	}
	return false, 0, nil
}

func (mockStrategy *MockStrategy) Block(ctx context.Context, key string, blockDuration time.Duration) error {
	mockStrategy.blockedKeys[key] = true
	return nil
}

func (mockStrategy *MockStrategy) Allow(ctx context.Context, key string, limit int64, windowDuration time.Duration) (limiter.Result, error) {
	isBlocked, timeToLive, err := mockStrategy.IsBlocked(ctx, key)
	if err != nil {
		return limiter.Result{}, err
	}
	if isBlocked {
		return limiter.Result{Allowed: false, RetryAfter: timeToLive}, nil
	}

	mockStrategy.counts[key]++
	currentCount := mockStrategy.counts[key]

	if currentCount > limit {
		mockStrategy.Block(ctx, key, 5*time.Minute)
		return limiter.Result{Allowed: false, RetryAfter: 5 * time.Minute}, nil
	}

	return limiter.Result{Allowed: true, Remaining: limit - currentCount}, nil
}

func buildTestConfig(ipLimit, tokenLimit int64, windowSeconds int64) middleware.Config {
	return middleware.Config{
		IPMaxRequests:    ipLimit,
		TokenMaxRequests: tokenLimit,
		WindowDuration:   time.Duration(windowSeconds) * time.Second,
	}
}

func buildHelloWorldHandler() http.Handler {
	return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		responseWriter.WriteHeader(http.StatusOK)
		responseWriter.Write([]byte("Hello, World!"))
	})
}

func TestFirstRequestsUpToLimitAreAllowed(t *testing.T) {
	config := buildTestConfig(3, 10, 1)
	mockStrategy := NewMockStrategy()
	rateLimitedHandler := middleware.RateLimiter(mockStrategy, config)(buildHelloWorldHandler())

	for requestNumber := 1; requestNumber <= 3; requestNumber++ {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.RemoteAddr = "127.0.0.1:12345"

		rateLimitedHandler.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusOK {
			t.Errorf("request %d: expected status 200, got %d", requestNumber, recorder.Code)
		}
	}
}

func TestRequestBeyondLimitIsBlocked(t *testing.T) {
	config := buildTestConfig(3, 10, 1)
	mockStrategy := NewMockStrategy()
	rateLimitedHandler := middleware.RateLimiter(mockStrategy, config)(buildHelloWorldHandler())

	for requestNumber := 1; requestNumber <= 3; requestNumber++ {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.RemoteAddr = "127.0.0.1:12345"
		rateLimitedHandler.ServeHTTP(recorder, request)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "127.0.0.1:12345"
	rateLimitedHandler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusTooManyRequests {
		t.Errorf("expected status 429, got %d", recorder.Code)
	}
}

func TestBlockedResponseBodyIsCorrect(t *testing.T) {
	config := buildTestConfig(3, 10, 1)
	mockStrategy := NewMockStrategy()
	rateLimitedHandler := middleware.RateLimiter(mockStrategy, config)(buildHelloWorldHandler())

	for requestNumber := 1; requestNumber <= 4; requestNumber++ {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.RemoteAddr = "127.0.0.1:12345"
		rateLimitedHandler.ServeHTTP(recorder, request)

		if requestNumber == 4 {
			expectedBody := "you have reached the maximum number of requests or actions allowed within a certain time frame"
			actualBody := recorder.Body.String()
			if actualBody != expectedBody {
				t.Errorf("expected body %q, got %q", expectedBody, actualBody)
			}
		}
	}
}

func TestRetryAfterHeaderIsSetOnBlocked(t *testing.T) {
	config := buildTestConfig(3, 10, 1)
	mockStrategy := NewMockStrategy()
	rateLimitedHandler := middleware.RateLimiter(mockStrategy, config)(buildHelloWorldHandler())

	for requestNumber := 1; requestNumber <= 4; requestNumber++ {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.RemoteAddr = "127.0.0.1:12345"
		rateLimitedHandler.ServeHTTP(recorder, request)

		if requestNumber == 4 {
			retryAfterHeader := recorder.Header().Get("Retry-After")
			if retryAfterHeader == "" {
				t.Error("expected Retry-After header to be set")
			}
		}
	}
}

func TestTokenOverridesIPLimit(t *testing.T) {
	config := buildTestConfig(3, 10, 1)
	mockStrategy := NewMockStrategy()
	rateLimitedHandler := middleware.RateLimiter(mockStrategy, config)(buildHelloWorldHandler())

	for i := 0; i < 3; i++ {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.RemoteAddr = "192.168.1.1:54321"
		rateLimitedHandler.ServeHTTP(recorder, request)
	}

	blockedRecorder := httptest.NewRecorder()
	blockedRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	blockedRequest.RemoteAddr = "192.168.1.1:54321"
	rateLimitedHandler.ServeHTTP(blockedRecorder, blockedRequest)
	if blockedRecorder.Code != http.StatusTooManyRequests {
		t.Fatalf("expected IP to be blocked (429) after exceeding limit, got %d", blockedRecorder.Code)
	}

	tokenRecorder := httptest.NewRecorder()
	tokenRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	tokenRequest.RemoteAddr = "192.168.1.1:54321"
	tokenRequest.Header.Set("API_KEY", "my-secret-token")
	rateLimitedHandler.ServeHTTP(tokenRecorder, tokenRequest)

	if tokenRecorder.Code != http.StatusOK {
		t.Errorf("token config must override IP: expected 200 from blocked IP when valid token is provided, got %d", tokenRecorder.Code)
	}
}
