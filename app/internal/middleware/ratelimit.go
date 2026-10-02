package middleware

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// rateLimitWindow is the fixed window the per-IP request counter is kept for.
const rateLimitWindow = time.Minute

// RateLimit caps requests per client IP using a fixed one-minute window
// counted in Redis (INCR + EXPIRE), shared across all replicas of the app.
func RateLimit(cache *redis.Client, requestsPerMinute int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			ip := clientIP(r)
			window := time.Now().Unix() / int64(rateLimitWindow/time.Second)
			key := fmt.Sprintf("ratelimit:%s:%d", ip, window)

			// INCR and EXPIRE go in one transaction so a key can never be left without a TTL.
			var incr *redis.IntCmd
			_, err := cache.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				incr = pipe.Incr(ctx, key)
				pipe.Expire(ctx, key, rateLimitWindow)
				return nil
			})
			if err != nil {
				// Redis being unavailable should not take the API down (fail open),
				// but the limit is off meanwhile — leave a trace.
				slog.Warn("rate limit skipped: redis unavailable", "error", err)
				next.ServeHTTP(w, r)
				return
			}
			count := incr.Val()

			if int(count) > requestsPerMinute {
				w.Header().Set("Retry-After", strconv.Itoa(int(rateLimitWindow/time.Second)))
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// clientIP relies on RemoteAddr only: RealIP (registered before this middleware)
// has already replaced it with the client address from the ingress controller.
// Reading X-Forwarded-For here directly would let any client pick an arbitrary
// key and bypass the limit.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
