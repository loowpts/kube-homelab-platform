package config

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	for _, k := range []string{"HTTP_PORT", "METRICS_PORT", "CACHE_TTL_SECONDS", "RATE_LIMIT_RPM"} {
		t.Setenv(k, "")
	}

	cfg := Load()

	if cfg.HTTPPort != "8080" || cfg.MetricsPort != "9090" {
		t.Errorf("ports = %s/%s, want 8080/9090", cfg.HTTPPort, cfg.MetricsPort)
	}
	if cfg.CacheTTL != 10*time.Second {
		t.Errorf("CacheTTL = %v, want 10s", cfg.CacheTTL)
	}
	if cfg.RateLimitRPM != 120 {
		t.Errorf("RateLimitRPM = %d, want 120", cfg.RateLimitRPM)
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("METRICS_PORT", "9191")
	t.Setenv("RATE_LIMIT_RPM", "30")

	cfg := Load()

	if cfg.MetricsPort != "9191" {
		t.Errorf("MetricsPort = %s, want 9191", cfg.MetricsPort)
	}
	if cfg.RateLimitRPM != 30 {
		t.Errorf("RateLimitRPM = %d, want 30", cfg.RateLimitRPM)
	}
}

func TestLoadInvalidIntUsesDefaultAndWarns(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	t.Setenv("RATE_LIMIT_RPM", "12O")

	cfg := Load()

	if cfg.RateLimitRPM != 120 {
		t.Errorf("RateLimitRPM = %d, want default 120", cfg.RateLimitRPM)
	}
	if !strings.Contains(logs.String(), "key=RATE_LIMIT_RPM") {
		t.Errorf("no warning about RATE_LIMIT_RPM in logs: %q", logs.String())
	}
}
