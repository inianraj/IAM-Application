package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"crud/internal/domain"
)

// execer is what both *sql.DB and *sql.Tx offer for statements.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// permissionFK is the foreign key from group_permissions to the permissions
// table.
const permissionFK = "group_permissions_permission_fkey"

// insertGroupPermissions gives the group the permissions (without duplicates).
// The permissions table is the list of valid keys: the foreign key rejects any
// key that is not in it, reported as domain.ErrInvalidGroup (400).
func insertGroupPermissions(ctx context.Context, q execer, groupID int64, permissions []string) error {
	for _, p := range permissions {
		_, err := q.ExecContext(ctx,
			`INSERT INTO group_permissions (group_id, permission) VALUES ($1, $2)`,
			groupID, p)
		if pgCode(err) == foreignKeyViolation && pgConstraint(err) == permissionFK {
			return fmt.Errorf("%w: unknown permission %q", domain.ErrInvalidGroup, p)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// insertAllPermissions gives the group every permission in the permissions
// table: what the built-in admin group has.
func insertAllPermissions(ctx context.Context, q execer, groupID int64) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO group_permissions (group_id, permission) SELECT $1, key FROM permissions`,
		groupID)
	return err
}

// Permissions returns the permissions of the group, sorted. It returns an
// empty slice (never nil) for a group without any.
func (r *GroupRepository) Permissions(ctx context.Context, groupID int64) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT permission FROM group_permissions WHERE group_id = $1 ORDER BY permission`, groupID)
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

// SetPermissions replaces the group's permissions in one transaction. It
// returns domain.ErrGroupNotFound if the group does not exist.
func (r *GroupRepository) SetPermissions(ctx context.Context, groupID int64, permissions []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() // no-op after Commit

	var id int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM groups WHERE id = $1 FOR UPDATE`, groupID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrGroupNotFound
	}
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM group_permissions WHERE group_id = $1`, groupID); err != nil {
		return err
	}
	if err := insertGroupPermissions(ctx, tx, groupID, permissions); err != nil {
		return err
	}

	return tx.Commit()
}

// ListPermissions returns the permissions table: every permission a group can
// be given, grouped by resource.
func (r *GroupRepository) ListPermissions(ctx context.Context) ([]domain.Permission, error) {
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
