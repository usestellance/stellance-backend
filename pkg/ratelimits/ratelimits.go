package ratelimiter

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	DefaultLimit = RateLimitConfig{
		Capacity:     70,
		RefillRate:   50,
		RefillPeriod: 20 * time.Minute,
	}

	StrictLimit = RateLimitConfig{
		Capacity:     100,
		RefillRate:   50,
		RefillPeriod: 20 * time.Minute,
	}

	AuthenticatedLimit = RateLimitConfig{
		Capacity:     200,
		RefillRate:   200,
		RefillPeriod: 30 * time.Minute,
	}
)

type RateLimitErrorResponse struct {
	Status   int    `json:"status"`
	Message  string `json:"message"`
	Error    string `json:"error,omitempty"`
	Duration string `json:"duration,omitempty"`
}

type RateLimiter struct {
	redis        *redis.Client
	keyPrefix    string
	fallbackMode bool
}

type RateLimitConfig struct {
	Capacity     int64
	RefillRate   int64
	RefillPeriod time.Duration
}

type RateLimitStore struct {
	Tokens         float64   `json:"tokens"`
	LastRefillTime time.Time `json:"last_refill"`
}

func NewRateLimiter(redis *redis.Client, keyPrefix string, fallbackMode bool) *RateLimiter {
	return &RateLimiter{
		redis:        redis,
		keyPrefix:    keyPrefix,
		fallbackMode: fallbackMode,
	}
}

func (guard *RateLimiter) TraceRequest(r *http.Request) string {
	if userId := r.Context().Value("user_id"); userId != nil {
		return "user:" + userId.(string)
	}

	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.Header.Get("X-Real-IP")
	}
	if ip == "" {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err == nil {
			ip = host
		} else {
			ip = r.RemoteAddr
		}
	}
	return "ip:" + ip
}

func (guard *RateLimiter) CheckStore(ctx context.Context, key string, config RateLimitConfig) (allowed bool, remaining int64, resetAt time.Time, err error) {
	script := redis.NewScript(`
		local key = KEYS[1]
		local capacity = tonumber(ARGV[1])
		local refill_rate = tonumber(ARGV[2])
		local refill_period = tonumber(ARGV[3])
		local now = tonumber(ARGV[4])
		local requested_tokens = tonumber(ARGV[5])

		local bucket_json = redis.call('GET', key)
		local bucket
		
		if bucket_json then
			bucket = cjson.decode(bucket_json)
		else
			-- Initialize new bucket
			bucket = {
				tokens = capacity,
				last_refill = now
			}
		end

		local time_passed = now - bucket.last_refill
		local periods_passed = math.floor(time_passed / refill_period)
		local tokens_to_add = periods_passed * refill_rate
		
		-- Refill bucket (cap at capacity)
		bucket.tokens = math.min(capacity, bucket.tokens + tokens_to_add)
		
		-- Update last refill time if we added tokens
		if periods_passed > 0 then
			bucket.last_refill = bucket.last_refill + (periods_passed * refill_period)
		end
		
		-- Check if we have enough tokens
		local allowed = bucket.tokens >= requested_tokens
		local remaining = bucket.tokens
		
		if allowed then
			bucket.tokens = bucket.tokens - requested_tokens
			remaining = bucket.tokens
		end

		redis.call('SET', key, cjson.encode(bucket))
		redis.call('EXPIRE', key, refill_period * 2) -- Expire after 2 periods of inactivity
		
		local next_refill = bucket.last_refill + refill_period
		
		return {allowed and 1 or 0, remaining, next_refill}

	`)

	now := time.Now().Unix()
	result, err := script.Run(ctx, guard.redis, []string{key},
		config.Capacity,
		config.RefillRate,
		int64(config.RefillPeriod.Seconds()),
		now,
		1,
	).Result()

	if err != nil {
		if guard.fallbackMode {
			return true, config.Capacity, time.Now().Add(config.RefillPeriod), err
		}
		return false, 0, time.Now(), err
	}

	values := result.([]interface{})
	allowed = values[0].(int64) == 1
	resetAt = time.Unix(values[2].(int64), 0)
	return allowed, remaining, resetAt, nil
}

func (guard *RateLimiter) TokenBucket(config RateLimitConfig) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			identifier := guard.TraceRequest(r)
			fmt.Println("The rate limit key =>>>> :", identifier)
			key := fmt.Sprintf("%s:store:%s", guard.keyPrefix, identifier)

			allowed, remaining, resetAt, err := guard.CheckStore(ctx, key, config)

			w.Header().Set("X-RateLimit-Limit", strconv.FormatInt(config.Capacity, 10))
			w.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetAt.Unix(), 10))

			if err != nil && !guard.fallbackMode {
				e := &RateLimitErrorResponse{
					Status:  http.StatusServiceUnavailable,
					Message: "oops, it seems you're doing too much right now...",
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				json.NewEncoder(w).Encode(e)
				return
			}

			if !allowed {
				w.Header().Set("Retry-After", strconv.FormatInt(int64(config.RefillPeriod.Seconds()), 10))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(&RateLimitErrorResponse{
					Status:   http.StatusTooManyRequests,
					Message:  "You have been temporarily banned for suspected activity",
					Error:    "Limit Exceeded",
					Duration: config.RefillPeriod.String(),
				})
				return
			}
			h.ServeHTTP(w, r)
		})
	}
}
