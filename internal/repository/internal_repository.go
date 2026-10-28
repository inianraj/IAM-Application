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

const taskColumns = "id, user_id, organisation_id, title, description, created_at, updated_at"

// scanTask maps sql.ErrNoRows to ErrTaskNotFound, and a user_id that
// references no user (only possible on insert) to ErrUserNotFound.
func scanTask(row interface{ Scan(...any) error }) (domain.Task, error) {
	var t domain.Task
	var organisationID sql.NullInt64 // NULL for owners without an organisation
	err := row.Scan(&t.ID, &t.UserID, &organisationID, &t.Title, &t.Description, &t.CreatedAt, &t.UpdatedAt)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		return t, domain.ErrTaskNotFound
	case pgCode(err) == foreignKeyViolation:
		return domain.Task{}, domain.ErrUserNotFound
	}

	t.OrganisationID = organisationID.Int64
	return t, err
}

// Create stores the record with the owner's organisation (t.OrganisationID,
// NULL when 0).
func (r *TaskRepository) Create(ctx context.Context, t domain.Task) (domain.Task, error) {
	organisationID := sql.NullInt64{Int64: t.OrganisationID, Valid: t.OrganisationID != 0}

	return scanTask(r.db.QueryRowContext(ctx,
		`INSERT INTO records (user_id, organisation_id, title, description) VALUES ($1, $2, $3, $4) RETURNING `+taskColumns,
		t.UserID, organisationID, t.Title, t.Description))
}

func (r *TaskRepository) GetByID(ctx context.Context, id int64) (domain.Task, error) {
	return scanTask(r.db.QueryRowContext(ctx,
		`SELECT `+taskColumns+` FROM records WHERE id = $1`, id))
}

// ListByUser returns the user's records ordered by id. It returns an empty
// slice (never nil) for a user with no records, and domain.ErrUserNotFound if
// the user does not exist.
func (r *TaskRepository) ListByUser(ctx context.Context, userID int64) ([]domain.Task, error) {
	// LEFT JOIN so a user without records still yields one row (with NULL
	// record columns), which tells them apart from a missing user.
	rows, err := r.db.QueryContext(ctx,
		`SELECT r.id, r.organisation_id, r.title, r.description, r.created_at, r.updated_at
		 FROM users u LEFT JOIN records r ON r.user_id = u.id
		 WHERE u.id = $1
		 ORDER BY r.id`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	found := false
	tasks := []domain.Task{}
	for rows.Next() {
		found = true

		var id, organisationID sql.NullInt64
		var title, description sql.NullString
		var createdAt, updatedAt sql.NullTime
		if err := rows.Scan(&id, &organisationID, &title, &description, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		if id.Valid {
			tasks = append(tasks, domain.Task{
				ID:             id.Int64,
				UserID:         userID,
				OrganisationID: organisationID.Int64,
				Title:          title.String,
				Description:    description.String,
				CreatedAt:      createdAt.Time,
				UpdatedAt:      updatedAt.Time,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if !found {
		return nil, domain.ErrUserNotFound
	}

	return tasks, nil
}

// ListByOrganisation returns every record of the organisation's users ordered
// by id (an empty slice, never nil, when there are none).
func (r *TaskRepository) ListByOrganisation(ctx context.Context, organisationID int64) ([]domain.Task, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+taskColumns+` FROM records WHERE organisation_id = $1 ORDER BY id`,
		organisationID)
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

// Update sets the record's title and description. The owner is not changed.
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
