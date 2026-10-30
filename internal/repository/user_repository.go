package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"crud/internal/domain"
)

const (
	uniqueViolation     = "23505"
	foreignKeyViolation = "23503"
)

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func pgCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

// pgConstraint returns the violated constraint's name, or "" if err is not a
// PostgreSQL constraint error.
func pgConstraint(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.ConstraintName
	}
	return ""
}

// userColumns are in scanUser order.
const userColumns = "id, name, email, group_id, organisation_id, signup_method, created_at, updated_at, password_hash"

// scanUser maps sql.ErrNoRows to ErrUserNotFound and a duplicate email to
// ErrEmailTaken.
func scanUser(row interface{ Scan(...any) error }) (domain.User, error) {
	var u domain.User
	var groupID, organisationID sql.NullInt64 // NULL for a user without an organisation
	err := row.Scan(&u.ID, &u.Name, &u.Email, &groupID, &organisationID, &u.SignupMethod, &u.CreatedAt, &u.UpdatedAt, &u.PasswordHash)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		return domain.User{}, domain.ErrUserNotFound
	case pgCode(err) == uniqueViolation:
		return domain.User{}, domain.ErrEmailTaken
	}

	u.GroupID = groupID.Int64
	u.OrganisationID = organisationID.Int64
	return u, err
}

// Create inserts the user into organisation u.OrganisationID with the given
// roles (the organisation's member role when roleIDs is empty) and no group.
// It returns domain.ErrEmailTaken for a duplicate email,
// domain.ErrOrganisationNotFound if the organisation does not exist, and
// domain.ErrRoleNotFound for a role that is missing or belongs to another
// organisation.
func (r *UserRepository) Create(ctx context.Context, u domain.User, roleIDs []int64) (domain.User, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.User{}, err
	}
	defer tx.Rollback() // no-op after Commit

	user, err := scanUser(tx.QueryRowContext(ctx,
		`INSERT INTO users (name, email, signup_method, organisation_id, password_hash)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING `+userColumns,
		u.Name, u.Email, u.SignupMethod, u.OrganisationID, u.PasswordHash))
	if pgCode(err) == foreignKeyViolation {
		return domain.User{}, domain.ErrOrganisationNotFound
	}
	if err != nil {
		return domain.User{}, err
	}

	if len(roleIDs) == 0 {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO user_roles (user_id, role_id, organisation_id)
			 SELECT $1, id, organisation_id FROM roles WHERE organisation_id = $2 AND name = $3`,
			user.ID, user.OrganisationID, domain.MemberRoleName)
		if err != nil {
			return domain.User{}, err
		}
		if n, err := res.RowsAffected(); err != nil {
			return domain.User{}, err
		} else if n == 0 {
			return domain.User{}, fmt.Errorf("%w: organisation has no %s role", domain.ErrRoleNotFound, domain.MemberRoleName)
		}
	} else if err := insertUserRoles(ctx, tx, user.ID, user.OrganisationID, roleIDs); err != nil {
		return domain.User{}, err
	}

	if err := tx.Commit(); err != nil {
		return domain.User{}, err
	}
	return user, nil
}

func (r *UserRepository) GetByID(ctx context.Context, id int64) (domain.User, error) {
	return scanUser(r.db.QueryRowContext(ctx,
		`SELECT `+userColumns+` FROM users WHERE id = $1`, id))
}

// GetByEmail is used by login. email must be lower-cased. Unknown email →
// ErrUserNotFound.
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (domain.User, error) {
	return scanUser(r.db.QueryRowContext(ctx,
		`SELECT `+userColumns+` FROM users WHERE email = $1`, email))
}

// Update sets the user's name. The email (the login identity), the group and
// the organisation are not changed.
func (r *UserRepository) Update(ctx context.Context, u domain.User) (domain.User, error) {
	return scanUser(r.db.QueryRowContext(ctx,
		`UPDATE users SET name = $1, updated_at = NOW()
		 WHERE id = $2
		 RETURNING `+userColumns,
		u.Name, u.ID))
}

// Delete removes the user. A user that still has records returns
// domain.ErrUserInUse, and the last owner of an organisation returns
// domain.ErrLastOwner.
func (r *UserRepository) Delete(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() // no-op after Commit

	var organisationID sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT organisation_id FROM users WHERE id = $1`, id).Scan(&organisationID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrUserNotFound
	}
	if err != nil {
		return err
	}

	if organisationID.Valid {
		// Locking the organisation row serialises owner changes, so two owners
		// cannot remove each other at the same time.
		if err := lockOrganisation(ctx, tx, organisationID.Int64); err != nil {
			return err
		}
		wasOwner, err := isOwner(ctx, tx, id)
		if err != nil {
			return err
		}
		if wasOwner {
			owners, err := countOwners(ctx, tx, organisationID.Int64)
			if err != nil {
				return err
			}
			if owners <= 1 {
				return domain.ErrLastOwner
			}
		}
	}

	res, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, id)
	if pgCode(err) == foreignKeyViolation {
		return domain.ErrUserInUse
	}
	if err != nil {
		return err
	}

	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrUserNotFound
	}

	return tx.Commit()
}

// AssignGroup moves the user to a group of the user's organisation (the
// organisation stored on the user), or removes the user from their group when
// groupID is 0. The check and the update are one statement, so they cannot
// race. It returns domain.ErrUserNotFound, domain.ErrGroupNotFound, or
// domain.ErrGroupNotInOrganisation when the update does not happen.
func (r *UserRepository) AssignGroup(ctx context.Context, userID, groupID int64) error {
	var res sql.Result
	var err error
	if groupID == 0 {
		res, err = r.db.ExecContext(ctx,
			`UPDATE users SET group_id = NULL, updated_at = NOW() WHERE id = $1`, userID)
	} else {
		res, err = r.db.ExecContext(ctx,
			`UPDATE users u SET group_id = g.id, updated_at = NOW()
			 FROM groups g
			 WHERE u.id = $2 AND g.id = $1 AND g.organisation_id = u.organisation_id`,
			groupID, userID)
	}
	if err != nil {
		return err
	}

	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}

	// Find out why nothing was updated.
	var userExists bool
	var groupOrg sql.NullInt64
	err = r.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM users WHERE id = $1),
		        (SELECT organisation_id FROM groups WHERE id = $2)`,
		userID, groupID).Scan(&userExists, &groupOrg)
	if err != nil {
		return err
	}

	switch {
	case !userExists:
		return domain.ErrUserNotFound
	case !groupOrg.Valid:
		return domain.ErrGroupNotFound
	default:
		return domain.ErrGroupNotInOrganisation
	}
}

// queryer is what both *sql.DB and *sql.Tx offer for single-row queries.
type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// lockOrganisation takes a row lock on the organisation until the transaction
// ends.
func lockOrganisation(ctx context.Context, q queryer, organisationID int64) error {
	var id int64
	return q.QueryRowContext(ctx, `SELECT id FROM organisations WHERE id = $1 FOR UPDATE`, organisationID).Scan(&id)
}

// isOwner reports whether the user holds the owner role directly.
func isOwner(ctx context.Context, q queryer, userID int64) (bool, error) {
	var owner bool
	err := q.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM user_roles ur JOIN roles r ON r.id = ur.role_id
		                WHERE ur.user_id = $1 AND r.name = $2)`,
		userID, domain.OwnerRoleName).Scan(&owner)
	return owner, err
}

// countOwners returns how many users of the organisation hold the owner role.
func countOwners(ctx context.Context, q queryer, organisationID int64) (int, error) {
	var n int
	err := q.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT ur.user_id) FROM user_roles ur JOIN roles r ON r.id = ur.role_id
		 WHERE r.organisation_id = $1 AND r.name = $2`,
		organisationID, domain.OwnerRoleName).Scan(&n)
	return n, err
}

// insertUserRoles gives the user the roles. The composite foreign keys
// guarantee both are in the same organisation; anything else is
// domain.ErrRoleNotFound.
func insertUserRoles(ctx context.Context, q execer, userID, organisationID int64, roleIDs []int64) error {
	for _, id := range roleIDs {
		_, err := q.ExecContext(ctx,
			`INSERT INTO user_roles (user_id, role_id, organisation_id) VALUES ($1, $2, $3)`,
			userID, id, organisationID)
		if pgCode(err) == foreignKeyViolation {
			return domain.ErrRoleNotFound
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// Roles returns the roles the user holds directly (not the ones from the
// group), oldest first; an empty slice, never nil, for a user without any.
func (r *UserRepository) Roles(ctx context.Context, userID int64) ([]domain.Role, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT r.id, r.organisation_id, r.name
		 FROM user_roles ur JOIN roles r ON r.id = ur.role_id
		 WHERE ur.user_id = $1 ORDER BY r.id`, userID)
	if err != nil {
		return nil, err
	}
	return scanRoles(rows)
}

// SetRoles replaces the roles the user holds directly, in one transaction. It
// returns domain.ErrUserNotFound if the user does not exist,
// domain.ErrRoleNotFound for a role that is missing or belongs to another
// organisation, and domain.ErrLastOwner when it would leave the organisation
// without an owner.
func (r *UserRepository) SetRoles(ctx context.Context, userID int64, roleIDs []int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() // no-op after Commit

	var organisationID sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT organisation_id FROM users WHERE id = $1`, userID).Scan(&organisationID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrUserNotFound
	}
	if err != nil {
		return err
	}
	if !organisationID.Valid {
		return domain.ErrOrganisationNotFound // roles belong to an organisation
	}

	// Serialise owner changes of the organisation (see Delete).
	if err := lockOrganisation(ctx, tx, organisationID.Int64); err != nil {
		return err
	}
	wasOwner, err := isOwner(ctx, tx, userID)
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM user_roles WHERE user_id = $1`, userID); err != nil {
		return err
	}
	if err := insertUserRoles(ctx, tx, userID, organisationID.Int64, roleIDs); err != nil {
		return err
	}

	if wasOwner {
		owners, err := countOwners(ctx, tx, organisationID.Int64)
		if err != nil {
			return err
		}
		if owners == 0 {
			return domain.ErrLastOwner
		}
	}

	return tx.Commit()
}

// ListByOrganisation returns the organisation's users, oldest first (an empty
// slice, never nil, if it has none).
func (r *UserRepository) ListByOrganisation(ctx context.Context, organisationID int64) ([]domain.User, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+userColumns+` FROM users WHERE organisation_id = $1 ORDER BY id`, organisationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := []domain.User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}
