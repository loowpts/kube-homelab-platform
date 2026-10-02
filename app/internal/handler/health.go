package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type HealthHandler struct {
	pool  *pgxpool.Pool
	cache *redis.Client
}

func NewHealthHandler(pool *pgxpool.Pool, cache *redis.Client) *HealthHandler {
	return &HealthHandler{pool: pool, cache: cache}
}

// Healthz is the liveness probe: the process is up and can serve HTTP.
// It intentionally does not touch Postgres/Redis so a transient dependency
// outage never triggers a pod restart loop.
func (h *HealthHandler) Healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// readinessTimeout bounds the Postgres and Redis pings so a hung dependency
// fails the probe instead of hanging it.
const readinessTimeout = 2 * time.Second

// Readyz is the readiness probe: only report ready when the app can
// actually serve traffic. Postgres is required; Redis is reported but does not
// fail the probe: without it the cache misses and the rate limit fails open,
// and the app keeps serving. Failing readiness on Redis would pull every pod
// out of the Service at once and turn a Redis outage into a full outage.
func (h *HealthHandler) Readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
	defer cancel()

	result := map[string]string{}
	ready := true

	if err := h.pool.Ping(ctx); err != nil {
		ready = false
		// /readyz is reachable through the Ingress, so details go to the log only
		slog.Warn("readiness check failed", "dependency", "postgres", "error", err)
		result["postgres"] = "unavailable"
	} else {
		result["postgres"] = "ok"
	}

	if err := h.cache.Ping(ctx).Err(); err != nil {
		// /readyz is reachable through the Ingress, so details go to the log only
		slog.Warn("readiness check degraded", "dependency", "redis", "error", err)
		result["redis"] = "unavailable"
	} else {
		result["redis"] = "ok"
	}

	status := http.StatusOK
	if !ready {
		status = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(result)
}
