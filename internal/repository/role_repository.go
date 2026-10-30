package repository

import (
	"context"
	"database/sql"
	"errors"

	"crud/internal/domain"
)

type RoleRepository struct {
	db *sql.DB
}

func NewRoleRepository(db *sql.DB) *RoleRepository {
	return &RoleRepository{db: db}
}

const roleColumns = "id, organisation_id, name"

// roleOrganisationFK is the foreign key from roles to organisations.
const roleOrganisationFK = "roles_organisation_id_fkey"

// scanRole maps sql.ErrNoRows to ErrRoleNotFound, a duplicate name within the
// organisation to ErrRoleExists, and an unknown organisation_id to
// ErrOrganisationNotFound.
func scanRole(row interface{ Scan(...any) error }) (domain.Role, error) {
	var r domain.Role
	err := row.Scan(&r.ID, &r.OrganisationID, &r.Name)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		return domain.Role{}, domain.ErrRoleNotFound
	case pgCode(err) == uniqueViolation:
		return domain.Role{}, domain.ErrRoleExists
	case pgCode(err) == foreignKeyViolation && pgConstraint(err) == roleOrganisationFK:
		return domain.Role{}, domain.ErrOrganisationNotFound
	}

	return r, err
}

// scanRoles reads a result set of roleColumns rows into a non-nil slice.
func scanRoles(rows *sql.Rows) ([]domain.Role, error) {
	defer rows.Close()

	roles := []domain.Role{}
	for rows.Next() {
		var r domain.Role
		if err := rows.Scan(&r.ID, &r.OrganisationID, &r.Name); err != nil {
			return nil, err
		}
		roles = append(roles, r)
	}
	return roles, rows.Err()
}

// Create inserts the role together with its permissions, in one transaction.
// An unknown permission key returns domain.ErrInvalidRole.
func (r *RoleRepository) Create(ctx context.Context, role domain.Role, permissions []string) (domain.Role, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Role{}, err
	}
	defer tx.Rollback() // no-op after Commit

	created, err := scanRole(tx.QueryRowContext(ctx,
		`INSERT INTO roles (organisation_id, name) VALUES ($1, $2) RETURNING `+roleColumns,
		role.OrganisationID, role.Name))
	if err != nil {
		return domain.Role{}, err
	}

	if err := insertRolePermissions(ctx, tx, created.ID, permissions); err != nil {
		return domain.Role{}, err
	}

	if err := tx.Commit(); err != nil {
		return domain.Role{}, err
	}
	return created, nil
}

func (r *RoleRepository) GetByID(ctx context.Context, id int64) (domain.Role, error) {
	return scanRole(r.db.QueryRowContext(ctx,
		`SELECT `+roleColumns+` FROM roles WHERE id = $1`, id))
}

// ListByOrganisation returns the organisation's roles ordered by id (an empty
// slice, never nil, if it has none).
func (r *RoleRepository) ListByOrganisation(ctx context.Context, organisationID int64) ([]domain.Role, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+roleColumns+` FROM roles WHERE organisation_id = $1 ORDER BY id`, organisationID)
	if err != nil {
		return nil, err
	}
	return scanRoles(rows)
}

// Update renames the role; its organisation never changes.
func (r *RoleRepository) Update(ctx context.Context, role domain.Role) (domain.Role, error) {
	return scanRole(r.db.QueryRowContext(ctx,
		`UPDATE roles SET name = $1 WHERE id = $2 RETURNING `+roleColumns,
		role.Name, role.ID))
}

// Delete removes the role. A role that a user or group still has returns
// domain.ErrRoleInUse.
func (r *RoleRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM roles WHERE id = $1`, id)
	if pgCode(err) == foreignKeyViolation {
		return domain.ErrRoleInUse
	}
	if err != nil {
		return err
	}

	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrRoleNotFound
	}
	return nil
}

// Permissions returns the permissions of the role, sorted (an empty slice,
// never nil, for a role without any).
func (r *RoleRepository) Permissions(ctx context.Context, roleID int64) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT permission_key FROM role_permissions WHERE role_id = $1 ORDER BY permission_key`, roleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	permissions := []string{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		permissions = append(permissions, p)
	}
	return permissions, rows.Err()
}

// SetPermissions replaces the role's permissions in one transaction. It
// returns domain.ErrRoleNotFound if the role does not exist and
// domain.ErrInvalidRole for an unknown permission key.
func (r *RoleRepository) SetPermissions(ctx context.Context, roleID int64, permissions []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() // no-op after Commit

	var id int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM roles WHERE id = $1 FOR UPDATE`, roleID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrRoleNotFound
	}
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM role_permissions WHERE role_id = $1`, roleID); err != nil {
		return err
	}
	if err := insertRolePermissions(ctx, tx, roleID, permissions); err != nil {
		return err
	}

	return tx.Commit()
}

// HolderIDs returns the ids of the users who have the role, directly or
// through their group. The cache uses it to drop their permissions.
func (r *RoleRepository) HolderIDs(ctx context.Context, roleID int64) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT user_id FROM user_roles WHERE role_id = $1
		 UNION
		 SELECT u.id FROM users u JOIN group_roles gr ON gr.group_id = u.group_id WHERE gr.role_id = $1`,
		roleID)
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

// ListPermissions returns the permissions table: every permission a role can
// be given.
func (r *RoleRepository) ListPermissions(ctx context.Context) ([]domain.Permission, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT key, resource, action, COALESCE(scope, ''), description
		 FROM permissions ORDER BY resource, key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []domain.Permission{}
	for rows.Next() {
		var p domain.Permission
		if err := rows.Scan(&p.Key, &p.Resource, &p.Action, &p.Scope, &p.Description); err != nil {
			return nil, err
		}
		list = append(list, p)
	}

	return list, rows.Err()
}
