package repository

import (
	"context"
	"database/sql"
	"errors"

	"crud/internal/domain"
)

type TaskRepository struct {
	db *sql.DB
}

func NewTaskRepository(db *sql.DB) *TaskRepository {
	return &TaskRepository{db: db}
}

const taskColumns = "id, title, description, created_at, updated_at"

func scanTask(row interface{ Scan(...any) error }) (domain.Task, error) {
	var t domain.Task
	err := row.Scan(&t.ID, &t.Title, &t.Description, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return t, domain.ErrTaskNotFound
	}
	return t, err
}

func (r *TaskRepository) Create(ctx context.Context, t domain.Task) (domain.Task, error) {
	return scanTask(r.db.QueryRowContext(ctx,
		`INSERT INTO records (title, description) VALUES ($1, $2) RETURNING `+taskColumns,
		t.Title, t.Description))
}

func (r *TaskRepository) GetByID(ctx context.Context, id int64) (domain.Task, error) {
	return scanTask(r.db.QueryRowContext(ctx,
		`SELECT `+taskColumns+` FROM records WHERE id = $1`, id))
}

func (r *TaskRepository) List(ctx context.Context) ([]domain.Task, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+taskColumns+` FROM records ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := []domain.Task{}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}

	return tasks, rows.Err()
}

func (r *TaskRepository) Update(ctx context.Context, t domain.Task) (domain.Task, error) {
	return scanTask(r.db.QueryRowContext(ctx,
		`UPDATE records SET title = $1, description = $2, updated_at = NOW()
		 WHERE id = $3 RETURNING `+taskColumns,
		t.Title, t.Description, t.ID))
}

func (r *TaskRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM records WHERE id = $1`, id)
	if err != nil {
		return err
	}

	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrTaskNotFound
	}

	return nil
}
