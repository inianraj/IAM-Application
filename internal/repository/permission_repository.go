package repository

import (
	"context"
	"database/sql"
	"fmt"

	"crud/internal/domain"
)

// execer is what both *sql.DB and *sql.Tx offer for statements.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// permissionFK is the foreign key from role_permissions to the permissions
// table.
const permissionFK = "role_permissions_permission_key_fkey"

// insertRolePermissions gives the role the permissions (without duplicates).
// The permissions table is the list of valid keys: the foreign key rejects any
// key that is not in it, reported as domain.ErrInvalidRole (400).
func insertRolePermissions(ctx context.Context, q execer, roleID int64, permissions []string) error {
	for _, p := range permissions {
		_, err := q.ExecContext(ctx,
			`INSERT INTO role_permissions (role_id, permission_key) VALUES ($1, $2)`,
			roleID, p)
		if pgCode(err) == foreignKeyViolation && pgConstraint(err) == permissionFK {
			return fmt.Errorf("%w: unknown permission %q", domain.ErrInvalidRole, p)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// Permissions returns what the user may do, sorted: the union of the
// permissions of the roles the user holds directly and of the roles of the
// user's group. It returns an empty slice (never nil) for a user with none.
func (r *UserRepository) Permissions(ctx context.Context, userID int64) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT DISTINCT rp.permission_key
		 FROM role_permissions rp
		 WHERE rp.role_id IN (
		     SELECT role_id FROM user_roles WHERE user_id = $1
		     UNION
		     SELECT gr.role_id FROM group_roles gr JOIN users u ON u.group_id = gr.group_id WHERE u.id = $1)
		 ORDER BY 1`, userID)
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
