package repository

import (
	"context"
	"database/sql"
	"errors"

	"crud/internal/domain"
)

type OrganisationRepository struct {
	db *sql.DB
}

func NewOrganisationRepository(db *sql.DB) *OrganisationRepository {
	return &OrganisationRepository{db: db}
}

const organisationColumns = "id, name"

// scanOrganisation maps sql.ErrNoRows to ErrOrganisationNotFound and a
// unique violation (duplicate name) to ErrOrganisationExists.
func scanOrganisation(row interface{ Scan(...any) error }) (domain.Organisation, error) {
	var o domain.Organisation
	err := row.Scan(&o.ID, &o.Name)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		return o, domain.ErrOrganisationNotFound
	case pgCode(err) == uniqueViolation:
		return domain.Organisation{}, domain.ErrOrganisationExists
	}

	return o, err
}

// Create inserts the organisation together with its default group, in one
// transaction so an organisation never exists without it.
func (r *OrganisationRepository) Create(ctx context.Context, o domain.Organisation) (domain.Organisation, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Organisation{}, err
	}
	defer tx.Rollback() // no-op after Commit

	o, err = scanOrganisation(tx.QueryRowContext(ctx,
		`INSERT INTO organisations (name) VALUES ($1) RETURNING `+organisationColumns,
		o.Name))
	if err != nil {
		return domain.Organisation{}, err
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO groups (organisation_id, name) VALUES ($1, $2)`,
		o.ID, domain.DefaultGroupName); err != nil {
		return domain.Organisation{}, err
	}

	if err := tx.Commit(); err != nil {
		return domain.Organisation{}, err
	}

	return o, nil
}

func (r *OrganisationRepository) GetByID(ctx context.Context, id int64) (domain.Organisation, error) {
	return scanOrganisation(r.db.QueryRowContext(ctx,
		`SELECT `+organisationColumns+` FROM organisations WHERE id = $1`, id))
}

func (r *OrganisationRepository) List(ctx context.Context) ([]domain.Organisation, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+organisationColumns+` FROM organisations ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	orgs := []domain.Organisation{}
	for rows.Next() {
		o, err := scanOrganisation(rows)
		if err != nil {
			return nil, err
		}
		orgs = append(orgs, o)
	}

	return orgs, rows.Err()
}

func (r *OrganisationRepository) Update(ctx context.Context, o domain.Organisation) (domain.Organisation, error) {
	return scanOrganisation(r.db.QueryRowContext(ctx,
		`UPDATE organisations SET name = $1 WHERE id = $2 RETURNING `+organisationColumns,
		o.Name, o.ID))
}

// Delete removes the organisation and its groups (the groups_organisation_id
// foreign key cascades). It fails with domain.ErrOrganisationInUse if any of
// those groups still has users.
func (r *OrganisationRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM organisations WHERE id = $1`, id)
	if pgCode(err) == foreignKeyViolation {
		return domain.ErrOrganisationInUse
	}
	if err != nil {
		return err
	}

	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrOrganisationNotFound
	}

	return nil
}
