package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"todo-app/internal/models"
)

var ErrNotFound = errors.New("task not found")

type TaskRepository struct {
	pool *pgxpool.Pool
}

func NewTaskRepository(pool *pgxpool.Pool) *TaskRepository {
	return &TaskRepository{pool: pool}
}

func (r *TaskRepository) List(ctx context.Context) ([]models.Task, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, title, description, done, created_at
		FROM tasks
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("query tasks: %w", err)
	}
	defer rows.Close()

	var tasks []models.Task
	for rows.Next() {
		var t models.Task
		if err := rows.Scan(&t.ID, &t.Title, &t.Description, &t.Done, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tasks: %w", err)
	}
	return tasks, nil
}

func (r *TaskRepository) Get(ctx context.Context, id int64) (models.Task, error) {
	var t models.Task
	err := r.pool.QueryRow(ctx, `
		SELECT id, title, description, done, created_at
		FROM tasks WHERE id = $1`, id).Scan(&t.ID, &t.Title, &t.Description, &t.Done, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Task{}, ErrNotFound
	}
	if err != nil {
		return models.Task{}, fmt.Errorf("get task: %w", err)
	}
	return t, nil
}

func (r *TaskRepository) Create(ctx context.Context, title, description string) (models.Task, error) {
	var t models.Task
	err := r.pool.QueryRow(ctx, `
		INSERT INTO tasks (title, description)
		VALUES ($1, $2)
		RETURNING id, title, description, done, created_at`, title, description,
	).Scan(&t.ID, &t.Title, &t.Description, &t.Done, &t.CreatedAt)
	if err != nil {
		return models.Task{}, fmt.Errorf("create task: %w", err)
	}
	return t, nil
}

func (r *TaskRepository) Update(ctx context.Context, id int64, title, description string, done bool) (models.Task, error) {
	var t models.Task
	err := r.pool.QueryRow(ctx, `
		UPDATE tasks SET title = $2, description = $3, done = $4
		WHERE id = $1
		RETURNING id, title, description, done, created_at`, id, title, description, done,
	).Scan(&t.ID, &t.Title, &t.Description, &t.Done, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Task{}, ErrNotFound
	}
	if err != nil {
		return models.Task{}, fmt.Errorf("update task: %w", err)
	}
	return t, nil
}

func (r *TaskRepository) Delete(ctx context.Context, id int64) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM tasks WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
