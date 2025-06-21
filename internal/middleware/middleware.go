package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"regexp"
	"runtime/debug"
	"strings"
	"time"

	"github.com/The-True-Hooha/stellance-backend.git/pkg/logger"
	ratelimiter "github.com/The-True-Hooha/stellance-backend.git/pkg/ratelimits"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type ContextKey string
type ErrorResponse struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
	Stack   string `json:"stack,omitempty"`
}

const (
	LoggerKey      ContextKey = "logger"
	CorrelationKey ContextKey = "correlation_id"
	RequestIDKey   ContextKey = "request_id"
	UserIDKey      ContextKey = "user_id"
	UserEmailKey   ContextKey = "user_email"
	RoleKey        ContextKey = "role"
)

func GetLoggerFromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(LoggerKey).(*slog.Logger); ok {
		return logger
	}
	return logger.Logger().With("warning", "missing logger in context")
}

func WriteLoggerToContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, LoggerKey, logger)
}

func LoggerMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		correlationId := r.Header.Get("X-Correlation-Id")
		requestId := r.Header.Get("X-Request-Id")

		if correlationId == "" {
			correlationId = uuid.New().String()
		}
		if requestId == "" {
			requestId = uuid.New().String()
		}

		w.Header().Set("X-Request-Id", requestId)
		w.Header().Set("X-Correlation-Id", correlationId)

		log := logger.Logger()
		log = log.With(
			slog.String("correlation_id", correlationId),
			slog.String("request_id", requestId),
		)

		ctx := WriteLoggerToContext(r.Context(), log)
		ctx = context.WithValue(ctx, RequestIDKey, requestId)

		rw := responseWriter(w)
		start := time.Now()
		path := sanitizePath(r.URL.Path)

		log.Info("request_started",
			slog.String("method", r.Method),
			slog.String("path", path),
			slog.String("remote_addr", sanitizeIP(r.RemoteAddr)),
			slog.String("user_agent", r.UserAgent()),
		)

		h.ServeHTTP(rw, r.WithContext(ctx))
		duration := time.Since(start)

		attrs := []slog.Attr{
			slog.String("method", r.Method),
			slog.String("path", path),
			slog.Int("status", rw.status),
			slog.Int64("duration_ms", duration.Milliseconds()),
			slog.Int("size", rw.size),
		}

		switch {
		case rw.status >= 500:
			log.LogAttrs(r.Context(), slog.LevelError, "request_completed", attrs...)
		case rw.status >= 400:
			log.LogAttrs(r.Context(), slog.LevelWarn, "request_completed", attrs...)
		default:
			log.LogAttrs(r.Context(), slog.LevelInfo, "request_completed", attrs...)
		}
	})
}

type responseWriterS struct {
	http.ResponseWriter
	status  int
	size    int
	written bool
}

func responseWriter(w http.ResponseWriter) *responseWriterS {
	return &responseWriterS{
		status:         http.StatusOK,
		ResponseWriter: w,
		size:           0,
		written:        false,
	}
}

func (responseWrapper *responseWriterS) WriteHeader(code int) {
	responseWrapper.status = code
	if !responseWrapper.written {
		responseWrapper.ResponseWriter.WriteHeader(code)
		responseWrapper.written = true
	}
}

func (rw *responseWriterS) Write(data []byte) (int, error) {
	if !rw.written {
		rw.written = true
	}
	size, err := rw.ResponseWriter.Write(data)
	rw.size += size
	return size, err
}

func ErrorHandlerMiddleware(next http.Handler) http.Handler {
	log := logger.Logger()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := responseWriter(w)
		defer func() {
			if err := recover(); err != nil {
				env := os.Getenv("STAGE")
				errResponse := ErrorResponse{
					Status:  http.StatusInternalServerError,
					Message: "Internal Server Error",
				}
				dev := env == "dev" || env != "prod"

				if dev {
					errResponse.Error = fmt.Sprintf("%v", err)
					errResponse.Stack = string(debug.Stack())
				} else {
					errResponse.Error = "An unexpected error occurred."
				}

				log.Error("[ERROR] Recovered from panic: %v\n%s", err, debug.Stack())

				responseBytes, _ := json.Marshal(errResponse)
				rw.Header().Set("Content-Type", "application/json")
				rw.WriteHeader(http.StatusInternalServerError)
				rw.Write(responseBytes)
			}

		}()
		next.ServeHTTP(rw, r)
	})
}

func sanitizeIP(ipAddress string) string {
	ip, _, _ := net.SplitHostPort(ipAddress)
	if ip == "" {
		ip = ipAddress
	}

	parts := strings.Split(ip, ".")
	if len(parts) == 4 {
		return fmt.Sprintf("%s.%s.*.*", parts[0], parts[1])
	}
	if strings.Contains(ip, ":") {
		parts := strings.Split(ip, ":")
		if len(parts) >= 2 {
			return fmt.Sprintf("%s:%s:*", parts[0], parts[1])
		}
	}
	return "[REDACTED]"
}

func sanitizePath(path string) string {
	sensitivePatterns := []struct {
		pattern     *regexp.Regexp
		replacement string
	}{
		{regexp.MustCompile(`/users/([^/]+)`), "/users/[REDACTED]"},
		{regexp.MustCompile(`/wallets/([^/]+)`), "/wallets/[REDACTED]"},
		{regexp.MustCompile(`/invoices/([^/]+)`), "/invoices/[REDACTED]"},
		{regexp.MustCompile(`/reset-password/([^/]+)`), "/reset-password/[REDACTED]"},
		{regexp.MustCompile(`/verify/([^/]+)`), "/verify/[REDACTED]"},
	}

	sanitized := path
	for _, sp := range sensitivePatterns {
		sanitized = sp.pattern.ReplaceAllString(sanitized, sp.replacement)
	}
	return sanitized
}

func RateLimitGuardMiddleware(redis *redis.Client) func(http.Handler) http.Handler {
	prefix := "stellance:rate_limit"
	limiter := ratelimiter.NewRateLimiter(redis, prefix, true)

	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var config ratelimiter.RateLimitConfig
			switch {
			case strings.HasPrefix(r.URL.Path, "/api/v1/auth/login"),
				strings.HasPrefix(r.URL.Path, "/api/v1/auth/signup"):
				fmt.Println("strict limit is been used")
				config = ratelimiter.StrictLimit
			case r.Context().Value("user_id") != nil:
				fmt.Println("the authenticated limit is been used")
				config = ratelimiter.AuthenticatedLimit
			default:
				config = ratelimiter.DefaultLimit
			}

			handlerWithRateLimit := limiter.TokenBucket(config)(h)

			handlerWithRateLimit.ServeHTTP(w, r)
		})
	}
}
