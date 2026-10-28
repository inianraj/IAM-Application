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

// Create inserts the organisation with its admin and member groups and its
// first admin user (admin.Name, admin.Email, admin.SignupMethod, placed in the
// admin group), all in one transaction so nothing is left behind when a step
// fails. It returns domain.ErrOrganisationExists for a duplicate name and
// domain.ErrEmailTaken when the admin's email already belongs to a user (the
// unique constraint decides, so there is no check-then-insert race).
func (r *OrganisationRepository) Create(ctx context.Context, o domain.Organisation, admin domain.User) (domain.Organisation, domain.User, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Organisation{}, domain.User{}, err
	}
	defer tx.Rollback() // no-op after Commit

	o, err = scanOrganisation(tx.QueryRowContext(ctx,
		`INSERT INTO organisations (name) VALUES ($1) RETURNING `+organisationColumns,
		o.Name))
	if err != nil {
		return domain.Organisation{}, domain.User{}, err
	}

	// The two built-in groups: admin gets every permission in the table,
	// member the fixed member set.
	var adminGroupID int64
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO groups (organisation_id, name) VALUES ($1, $2) RETURNING id`,
		o.ID, domain.AdminGroupName).Scan(&adminGroupID); err != nil {
		return domain.Organisation{}, domain.User{}, err
	}
	if err := insertAllPermissions(ctx, tx, adminGroupID); err != nil {
		return domain.Organisation{}, domain.User{}, err
	}

	var memberGroupID int64
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO groups (organisation_id, name) VALUES ($1, $2) RETURNING id`,
		o.ID, domain.MemberGroupName).Scan(&memberGroupID); err != nil {
		return domain.Organisation{}, domain.User{}, err
	}
	if err := insertGroupPermissions(ctx, tx, memberGroupID, domain.MemberPermissions()); err != nil {
		return domain.Organisation{}, domain.User{}, err
	}

	created, err := scanUser(tx.QueryRowContext(ctx,
		`INSERT INTO users (name, email, signup_method, group_id, organisation_id)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING `+userColumns,
		admin.Name, admin.Email, admin.SignupMethod, adminGroupID, o.ID))
	if err != nil {
		return domain.Organisation{}, domain.User{}, err
	}

	if err := tx.Commit(); err != nil {
		return domain.Organisation{}, domain.User{}, err
	}

	return o, created, nil
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

// Delete removes the organisation with everything in it, in one transaction:
// the records of its users, then its users, then the organisation itself (its
// groups go with it, the groups_organisation_id foreign key cascades). It
// returns the ids of the removed users and records. A user created in the
// organisation while the delete runs makes the last step fail on the
// users.group_id foreign key: domain.ErrOrganisationInUse, nothing is removed,
// and the call can be retried.
func (r *OrganisationRepository) Delete(ctx context.Context, id int64) (domain.DeletedOrganisation, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.DeletedOrganisation{}, err
	}
	defer tx.Rollback() // no-op after Commit

	recordIDs, err := deleteReturningIDs(ctx, tx,
		`DELETE FROM records WHERE user_id IN (SELECT id FROM users WHERE organisation_id = $1) RETURNING id`, id)
	if err != nil {
		return domain.DeletedOrganisation{}, err
	}
	userIDs, err := deleteReturningIDs(ctx, tx,
		`DELETE FROM users WHERE organisation_id = $1 RETURNING id`, id)
	if err != nil {
		return domain.DeletedOrganisation{}, err
	}

	res, err := tx.ExecContext(ctx, `DELETE FROM organisations WHERE id = $1`, id)
	if pgCode(err) == foreignKeyViolation {
		return domain.DeletedOrganisation{}, domain.ErrOrganisationInUse
	}
	if err != nil {
		return domain.DeletedOrganisation{}, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return domain.DeletedOrganisation{}, err
	}
	if n == 0 {
		return domain.DeletedOrganisation{}, domain.ErrOrganisationNotFound
	}

	if err := tx.Commit(); err != nil {
		return domain.DeletedOrganisation{}, err
	}

	return domain.DeletedOrganisation{UserIDs: userIDs, RecordIDs: recordIDs}, nil
}

// deleteReturningIDs runs a DELETE ... RETURNING id statement and collects
// the ids.
func deleteReturningIDs(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
