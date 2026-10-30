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

// builtinRoles lists the four roles every organisation gets and what each one
// allows: owner has every permission in the permissions table, admin all but
// deleting the organisation.
var builtinRoles = []struct {
	name        string
	permissions func(ctx context.Context, q execer, roleID int64) error
}{
	{domain.OwnerRoleName, func(ctx context.Context, q execer, roleID int64) error {
		_, err := q.ExecContext(ctx,
			`INSERT INTO role_permissions (role_id, permission_key) SELECT $1, key FROM permissions`, roleID)
		return err
	}},
	{domain.AdminRoleName, func(ctx context.Context, q execer, roleID int64) error {
		_, err := q.ExecContext(ctx,
			`INSERT INTO role_permissions (role_id, permission_key) SELECT $1, key FROM permissions WHERE key <> $2`,
			roleID, domain.PermOrganisationErase)
		return err
	}},
	{domain.MemberRoleName, func(ctx context.Context, q execer, roleID int64) error {
		return insertRolePermissions(ctx, q, roleID, domain.MemberPermissions())
	}},
	{domain.GuestRoleName, func(ctx context.Context, q execer, roleID int64) error {
		return insertRolePermissions(ctx, q, roleID, domain.GuestPermissions())
	}},
}

// Create inserts the organisation with its four built-in roles (owner, admin,
// member, guest) and its first user (admin.Name, admin.Email,
// admin.SignupMethod), who gets the owner role, all in one transaction so
// nothing is left behind when a step fails. It returns
// domain.ErrOrganisationExists for a duplicate name and domain.ErrEmailTaken
// when the owner's email already belongs to a user (the unique constraint
// decides, so there is no check-then-insert race).
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

	var ownerRoleID int64
	for _, br := range builtinRoles {
		var roleID int64
		if err := tx.QueryRowContext(ctx,
			`INSERT INTO roles (organisation_id, name) VALUES ($1, $2) RETURNING id`,
			o.ID, br.name).Scan(&roleID); err != nil {
			return domain.Organisation{}, domain.User{}, err
		}
		if err := br.permissions(ctx, tx, roleID); err != nil {
			return domain.Organisation{}, domain.User{}, err
		}
		if br.name == domain.OwnerRoleName {
			ownerRoleID = roleID
		}
	}

	created, err := scanUser(tx.QueryRowContext(ctx,
		`INSERT INTO users (name, email, signup_method, organisation_id, password_hash)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING `+userColumns,
		admin.Name, admin.Email, admin.SignupMethod, o.ID, admin.PasswordHash))
	if err != nil {
		return domain.Organisation{}, domain.User{}, err
	}
	if err := insertUserRoles(ctx, tx, created.ID, o.ID, []int64{ownerRoleID}); err != nil {
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
