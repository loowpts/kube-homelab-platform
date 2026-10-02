//go:build integration

// Интеграционные тесты против настоящих Postgres и Redis.
//
//	TEST_POSTGRES_DSN — подключение с правом CREATE DATABASE (тест создаёт и удаляет свою БД)
//	TEST_REDIS_ADDR   — адрес Redis (используется БД 15, очищается до и после)
//
// Локально:
//
//	docker compose up -d postgres redis
//	TEST_POSTGRES_DSN='postgres://todoapp:todoapp@localhost:5432/todoapp?sslmode=disable' \
//	TEST_REDIS_ADDR=localhost:6379 go test -tags integration ./internal/integration/
package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"todo-app/internal/db"
	"todo-app/internal/handler"
	"todo-app/internal/middleware"
	"todo-app/internal/models"
	"todo-app/internal/repository"
	"todo-app/internal/service"
)

const (
	redisTestDB    = 15
	listCacheKey   = "tasks:list"
	startupTimeout = 60 * time.Second
)

var (
	pool    *pgxpool.Pool
	rdb     *redis.Client
	testDSN string
)

func TestMain(m *testing.M) {
	adminDSN := os.Getenv("TEST_POSTGRES_DSN")
	redisAddr := os.Getenv("TEST_REDIS_ADDR")
	if adminDSN == "" || redisAddr == "" {
		fmt.Println("TEST_POSTGRES_DSN и TEST_REDIS_ADDR не заданы — интеграционные тесты пропущены")
		os.Exit(0)
	}
	os.Exit(run(m, adminDSN, redisAddr))
}

func run(m *testing.M, adminDSN, redisAddr string) int {
	ctx := context.Background()

	// Postgres в CI стартует параллельно с джобой — ждём готовности, а не sleep.
	admin, err := waitPostgres(ctx, adminDSN)
	if err != nil {
		fmt.Println("postgres недоступен:", err)
		return 1
	}
	defer admin.Close(ctx)

	dbName := fmt.Sprintf("itest_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+dbName); err != nil {
		fmt.Println("create database:", err)
		return 1
	}
	defer func() {
		if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+dbName+" WITH (FORCE)"); err != nil {
			fmt.Println("drop database:", err)
		}
	}()

	u, err := url.Parse(adminDSN)
	if err != nil {
		fmt.Println("parse dsn:", err)
		return 1
	}
	u.Path = "/" + dbName
	testDSN = u.String()

	if err := db.RunMigrations(testDSN); err != nil {
		fmt.Println("migrations:", err)
		return 1
	}
	pool, err = db.NewPool(ctx, testDSN)
	if err != nil {
		fmt.Println("pool:", err)
		return 1
	}
	defer pool.Close()

	rdb = redis.NewClient(&redis.Options{Addr: redisAddr, DB: redisTestDB})
	defer rdb.Close()
	if err := waitRedis(ctx, rdb); err != nil {
		fmt.Println("redis недоступен:", err)
		return 1
	}
	defer rdb.FlushDB(ctx)

	return m.Run()
}

func waitPostgres(ctx context.Context, dsn string) (*pgx.Conn, error) {
	deadline := time.Now().Add(startupTimeout)
	for {
		conn, err := pgx.Connect(ctx, dsn)
		if err == nil {
			if err = conn.Ping(ctx); err == nil {
				return conn, nil
			}
			conn.Close(ctx)
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(time.Second)
	}
}

func waitRedis(ctx context.Context, c *redis.Client) error {
	deadline := time.Now().Add(startupTimeout)
	for {
		err := c.Ping(ctx).Err()
		if err == nil || time.Now().After(deadline) {
			return err
		}
		time.Sleep(time.Second)
	}
}

// reset — чистое состояние перед каждым тестом.
func reset(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE tasks RESTART IDENTITY"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if err := rdb.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("flushdb: %v", err)
	}
}

func TestMigrationsAreIdempotent(t *testing.T) {
	if err := db.RunMigrations(testDSN); err != nil {
		t.Fatalf("second run: %v", err)
	}
	var version int
	var dirty bool
	if err := pool.QueryRow(context.Background(), "SELECT version, dirty FROM schema_migrations").Scan(&version, &dirty); err != nil {
		t.Fatalf("schema_migrations: %v", err)
	}
	if version != 1 || dirty {
		t.Errorf("schema_migrations = (%d, dirty=%v), want (1, false)", version, dirty)
	}
}

func TestRepositoryCRUD(t *testing.T) {
	reset(t)
	ctx := context.Background()
	repo := repository.NewTaskRepository(pool)

	created, err := repo.Create(ctx, "buy milk", "2 liters")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == 0 || created.Done || created.CreatedAt.IsZero() {
		t.Errorf("created = %+v, want id, done=false, created_at", created)
	}

	got, err := repo.Get(ctx, created.ID)
	if err != nil || got.Title != "buy milk" {
		t.Fatalf("get = %+v, %v", got, err)
	}

	updated, err := repo.Update(ctx, created.ID, "buy milk", "2 liters", true)
	if err != nil || !updated.Done {
		t.Fatalf("update = %+v, %v", updated, err)
	}

	list, err := repo.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %v, %v", list, err)
	}

	if err := repo.Delete(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := repo.Get(ctx, created.ID); err != repository.ErrNotFound {
		t.Errorf("get after delete: err = %v, want ErrNotFound", err)
	}
	if err := repo.Delete(ctx, created.ID); err != repository.ErrNotFound {
		t.Errorf("delete missing: err = %v, want ErrNotFound", err)
	}
}

func TestServiceListCacheIsFilledAndInvalidated(t *testing.T) {
	reset(t)
	ctx := context.Background()
	svc := service.NewTaskService(repository.NewTaskRepository(pool), rdb, time.Minute)

	if _, err := svc.Create(ctx, "first", ""); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.List(ctx); err != nil {
		t.Fatalf("list: %v", err)
	}

	raw, err := rdb.Get(ctx, listCacheKey).Bytes()
	if err != nil {
		t.Fatalf("cache after list: %v", err)
	}
	var cached []models.Task
	if err := json.Unmarshal(raw, &cached); err != nil || len(cached) != 1 {
		t.Fatalf("cached = %s, %v", raw, err)
	}
	if ttl := rdb.TTL(ctx, listCacheKey).Val(); ttl <= 0 || ttl > time.Minute {
		t.Errorf("cache TTL = %v, want (0, 1m]", ttl)
	}

	if _, err := svc.Create(ctx, "second", ""); err != nil {
		t.Fatalf("create: %v", err)
	}
	if n := rdb.Exists(ctx, listCacheKey).Val(); n != 0 {
		t.Errorf("cache must be invalidated after create, exists = %d", n)
	}
}

func TestRateLimitWithRedis(t *testing.T) {
	reset(t)
	const limit = 3

	h := middleware.RateLimit(rdb, limit)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 1; i <= limit+1; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
		req.RemoteAddr = "203.0.113.10:5000"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if i <= limit && rec.Code != http.StatusOK {
			t.Fatalf("request %d: status %d, want 200", i, rec.Code)
		}
		if i == limit+1 {
			if rec.Code != http.StatusTooManyRequests {
				t.Fatalf("request %d: status %d, want 429", i, rec.Code)
			}
			if rec.Header().Get("Retry-After") != "60" {
				t.Errorf("Retry-After = %q, want 60", rec.Header().Get("Retry-After"))
			}
		}
	}

	keys := rdb.Keys(context.Background(), "ratelimit:203.0.113.10:*").Val()
	if len(keys) != 1 {
		t.Fatalf("rate limit keys = %v, want 1", keys)
	}
	if ttl := rdb.TTL(context.Background(), keys[0]).Val(); ttl <= 0 {
		t.Errorf("rate limit key TTL = %v, want > 0 (ключ не должен жить вечно)", ttl)
	}
}

func TestAPIEndToEnd(t *testing.T) {
	reset(t)
	api := handler.NewTaskAPIHandler(service.NewTaskService(repository.NewTaskRepository(pool), rdb, time.Minute))
	r := chi.NewRouter()
	r.Route("/api/tasks", func(rt chi.Router) {
		rt.Get("/", api.List)
		rt.Post("/", api.Create)
		rt.Get("/{id}", api.Get)
		rt.Put("/{id}", api.Update)
		rt.Delete("/{id}", api.Delete)
	})
	srv := httptest.NewServer(r)
	defer srv.Close()

	do := func(method, path string, body any) (*http.Response, models.Task) {
		t.Helper()
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		req, _ := http.NewRequest(method, srv.URL+path, &buf)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		defer resp.Body.Close()
		var task models.Task
		_ = json.NewDecoder(resp.Body).Decode(&task)
		return resp, task
	}

	resp, created := do(http.MethodPost, "/api/tasks/", map[string]any{"title": "e2e", "description": "via http"})
	if resp.StatusCode != http.StatusCreated || created.ID == 0 {
		t.Fatalf("POST: %d %+v", resp.StatusCode, created)
	}
	path := fmt.Sprintf("/api/tasks/%d", created.ID)

	if resp, got := do(http.MethodGet, path, nil); resp.StatusCode != http.StatusOK || got.Title != "e2e" {
		t.Fatalf("GET: %d %+v", resp.StatusCode, got)
	}
	if resp, got := do(http.MethodPut, path, map[string]any{"title": "e2e", "done": true}); resp.StatusCode != http.StatusOK || !got.Done {
		t.Fatalf("PUT: %d %+v", resp.StatusCode, got)
	}
	if resp, _ := do(http.MethodPost, "/api/tasks/", map[string]any{"title": ""}); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("POST without title: %d, want 400", resp.StatusCode)
	}
	if resp, _ := do(http.MethodDelete, path, nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE: %d", resp.StatusCode)
	}
	if resp, _ := do(http.MethodGet, path, nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET after delete: %d, want 404", resp.StatusCode)
	}
}

func TestReadyzWithLiveDependencies(t *testing.T) {
	h := handler.NewHealthHandler(pool, rdb)
	rec := httptest.NewRecorder()
	h.Readyz(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("readyz: %d %s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["postgres"] != "ok" || body["redis"] != "ok" {
		t.Errorf("readyz body = %v", body)
	}
}

// Redis недоступен: под остаётся ready (Postgres доступен), в ответе redis=unavailable.
func TestReadyzWithRedisDown(t *testing.T) {
	down := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1})
	defer down.Close()

	h := handler.NewHealthHandler(pool, down)
	rec := httptest.NewRecorder()
	h.Readyz(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("readyz with redis down: %d %s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["postgres"] != "ok" || body["redis"] != "unavailable" {
		t.Errorf("readyz body = %v", body)
	}
}
