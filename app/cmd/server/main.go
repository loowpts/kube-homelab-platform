package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"todo-app/internal/cache"
	"todo-app/internal/config"
	"todo-app/internal/db"
	"todo-app/internal/handler"
	"todo-app/internal/middleware"
	"todo-app/internal/repository"
	"todo-app/internal/service"
)

// Таймауты HTTP-серверов: защита от медленных клиентов и зависших соединений.
const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 15 * time.Second
	writeTimeout      = 15 * time.Second
	idleTimeout       = 60 * time.Second
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	migrateOnly := flag.Bool("migrate-only", false, "apply database migrations and exit")
	flag.Parse()

	cfg := config.Load()

	if *migrateOnly {
		slog.Info("applying database migrations")
		if err := db.RunMigrations(cfg.PostgresDSN); err != nil {
			slog.Error("migration failed", "error", err)
			os.Exit(1)
		}
		slog.Info("migrations applied successfully")
		return
	}

	if err := run(cfg); err != nil {
		slog.Error("server exited with error", "error", err)
		os.Exit(1)
	}
}

func run(cfg config.Config) error {
	ctx := context.Background()

	pool, err := db.NewPool(ctx, cfg.PostgresDSN)
	if err != nil {
		return err
	}
	defer pool.Close()

	redisClient := cache.NewClient(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	defer redisClient.Close()

	taskRepo := repository.NewTaskRepository(pool)
	taskSvc := service.NewTaskService(taskRepo, redisClient, cfg.CacheTTL)

	apiHandler := handler.NewTaskAPIHandler(taskSvc)
	webHandler, err := handler.NewWebHandler(taskSvc, "web/templates/*.html")
	if err != nil {
		return err
	}
	healthHandler := handler.NewHealthHandler(pool, redisClient)

	router := chi.NewRouter()
	router.Use(chimiddleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(chimiddleware.Logger)
	// Metrics снаружи Recoverer: паника превращается в 500 внутри и тоже попадает в метрики
	router.Use(middleware.Metrics)
	router.Use(chimiddleware.Recoverer)

	router.Get("/healthz", healthHandler.Healthz)
	router.Get("/readyz", healthHandler.Readyz)

	fileServer := http.FileServer(http.Dir("web/static"))
	router.Handle("/static/*", http.StripPrefix("/static/", fileServer))

	router.Get("/", webHandler.List)
	router.Get("/tasks/new", webHandler.NewForm)
	router.Post("/tasks", webHandler.Create)
	router.Get("/tasks/{id}/edit", webHandler.EditForm)
	router.Post("/tasks/{id}", webHandler.Update)
	router.Post("/tasks/{id}/delete", webHandler.Delete)

	router.Route("/api/tasks", func(api chi.Router) {
		api.Use(middleware.RateLimit(redisClient, cfg.RateLimitRPM))
		api.Get("/", apiHandler.List)
		api.Post("/", apiHandler.Create)
		api.Get("/{id}", apiHandler.Get)
		api.Put("/{id}", apiHandler.Update)
		api.Delete("/{id}", apiHandler.Delete)
	})

	srv := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           router,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	// Метрики на отдельном порту: он не публикуется через Ingress,
	// Prometheus ходит в него напрямую через ServiceMonitor.
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())
	metricsSrv := &http.Server{
		Addr:              ":" + cfg.MetricsPort,
		Handler:           metricsMux,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	serverErr := make(chan error, 2)
	go func() {
		slog.Info("starting http server", "port", cfg.HTTPPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()
	go func() {
		slog.Info("starting metrics server", "port", cfg.MetricsPort)
		if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		return err
	case sig := <-stop:
		slog.Info("shutting down", "signal", sig.String())
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := metricsSrv.Shutdown(shutdownCtx); err != nil {
		slog.Warn("metrics server shutdown failed", "error", err)
	}
	return srv.Shutdown(shutdownCtx)
}
