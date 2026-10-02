package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"todo-app/internal/models"
	"todo-app/internal/repository"
)

const tasksListCacheKey = "tasks:list"

type TaskService struct {
	repo     *repository.TaskRepository
	cache    *redis.Client
	cacheTTL time.Duration
}

func NewTaskService(repo *repository.TaskRepository, cache *redis.Client, cacheTTL time.Duration) *TaskService {
	return &TaskService{repo: repo, cache: cache, cacheTTL: cacheTTL}
}

func (s *TaskService) List(ctx context.Context) ([]models.Task, error) {
	if cached, ok := s.readListCache(ctx); ok {
		return cached, nil
	}

	tasks, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}

	s.writeListCache(ctx, tasks)
	return tasks, nil
}

func (s *TaskService) Get(ctx context.Context, id int64) (models.Task, error) {
	return s.repo.Get(ctx, id)
}

func (s *TaskService) Create(ctx context.Context, title, description string) (models.Task, error) {
	t, err := s.repo.Create(ctx, title, description)
	if err == nil {
		s.invalidateListCache(ctx)
	}
	return t, err
}

func (s *TaskService) Update(ctx context.Context, id int64, title, description string, done bool) (models.Task, error) {
	t, err := s.repo.Update(ctx, id, title, description, done)
	if err == nil {
		s.invalidateListCache(ctx)
	}
	return t, err
}

func (s *TaskService) Delete(ctx context.Context, id int64) error {
	err := s.repo.Delete(ctx, id)
	if err == nil {
		s.invalidateListCache(ctx)
	}
	return err
}

func (s *TaskService) readListCache(ctx context.Context) ([]models.Task, bool) {
	raw, err := s.cache.Get(ctx, tasksListCacheKey).Bytes()
	if err != nil {
		if err != redis.Nil {
			slog.Warn("redis cache read failed", "error", err)
		}
		return nil, false
	}

	var tasks []models.Task
	if err := json.Unmarshal(raw, &tasks); err != nil {
		slog.Warn("redis cache unmarshal failed", "error", err)
		return nil, false
	}
	return tasks, true
}

func (s *TaskService) writeListCache(ctx context.Context, tasks []models.Task) {
	raw, err := json.Marshal(tasks)
	if err != nil {
		slog.Warn("redis cache marshal failed", "error", err)
		return
	}
	if err := s.cache.Set(ctx, tasksListCacheKey, raw, s.cacheTTL).Err(); err != nil {
		slog.Warn("redis cache write failed", "error", err)
	}
}

func (s *TaskService) invalidateListCache(ctx context.Context) {
	if err := s.cache.Del(ctx, tasksListCacheKey).Err(); err != nil {
		slog.Warn("redis cache invalidate failed", "error", err)
	}
}
