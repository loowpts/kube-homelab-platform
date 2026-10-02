package config

import (
	"log/slog"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPPort        string
	MetricsPort     string
	PostgresDSN     string
	RedisAddr       string
	RedisPassword   string
	RedisDB         int
	CacheTTL        time.Duration
	RateLimitRPM    int
	ShutdownTimeout time.Duration
}

func Load() Config {
	return Config{
		HTTPPort:        getEnv("HTTP_PORT", "8080"),
		MetricsPort:     getEnv("METRICS_PORT", "9090"),
		PostgresDSN:     getEnv("POSTGRES_DSN", "postgres://postgres:postgres@localhost:5432/todoapp?sslmode=disable"),
		RedisAddr:       getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:   getEnv("REDIS_PASSWORD", ""),
		RedisDB:         getEnvInt("REDIS_DB", 0),
		CacheTTL:        time.Duration(getEnvInt("CACHE_TTL_SECONDS", 10)) * time.Second,
		RateLimitRPM:    getEnvInt("RATE_LIMIT_RPM", 120),
		ShutdownTimeout: time.Duration(getEnvInt("SHUTDOWN_TIMEOUT_SECONDS", 15)) * time.Second,
	}
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		// Опечатка в конфиге не должна молча превращаться в значение по умолчанию
		slog.Warn("invalid integer in environment, using default", "key", key, "value", v, "default", fallback)
		return fallback
	}
	return i
}
