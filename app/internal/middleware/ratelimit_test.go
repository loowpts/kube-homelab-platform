package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/redis/go-redis/v9"
)

// Redis недоступен: запрос проходит (fail open), в логе остаётся предупреждение.
func TestRateLimitFailsOpenWhenRedisDown(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	// Порт 1 на localhost закрыт — соединение отклоняется сразу
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })

	h := RateLimit(client, 1)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/tasks/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (fail open)", rec.Code)
	}
	if !strings.Contains(logs.String(), "rate limit skipped") {
		t.Errorf("no warning in logs: %q", logs.String())
	}
}
