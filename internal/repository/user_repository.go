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
const userColumns = "id, name, email, group_id, organisation_id, signup_method, created_at, updated_at"

// scanUser maps sql.ErrNoRows to ErrUserNotFound and a duplicate email to
// ErrEmailTaken.
func scanUser(row interface{ Scan(...any) error }) (domain.User, error) {
	var u domain.User
	var groupID, organisationID sql.NullInt64 // NULL for a user without an organisation
	err := row.Scan(&u.ID, &u.Name, &u.Email, &groupID, &organisationID, &u.SignupMethod, &u.CreatedAt, &u.UpdatedAt)

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

// Create inserts the user into the member group of organisation
// u.OrganisationID. It returns domain.ErrEmailTaken for a duplicate email,
// domain.ErrOrganisationNotFound if the organisation does not exist, and
// domain.ErrGroupNotFound if it has no member group.
func (r *UserRepository) Create(ctx context.Context, u domain.User) (domain.User, error) {
	user, err := scanUser(r.db.QueryRowContext(ctx,
		`INSERT INTO users (name, email, signup_method, group_id, organisation_id)
		 SELECT $1, $2, $3::text, g.id, g.organisation_id FROM groups g
		 WHERE g.organisation_id = $4 AND g.name = $5
		 RETURNING `+userColumns,
		u.Name, u.Email, u.SignupMethod, u.OrganisationID, domain.MemberGroupName))
	if !errors.Is(err, domain.ErrUserNotFound) {
		return user, err
	}

	// No row was inserted: the organisation is missing, or it has no member
	// group.
	var orgExists bool
	err = r.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM organisations WHERE id = $1)`,
		u.OrganisationID).Scan(&orgExists)
	if err != nil {
		return domain.User{}, err
	}
	if !orgExists {
		return domain.User{}, domain.ErrOrganisationNotFound
	}

	return domain.User{}, fmt.Errorf("%w: organisation has no %s group", domain.ErrGroupNotFound, domain.MemberGroupName)
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
// domain.ErrUserInUse.
func (r *UserRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, id)
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

	return nil
}

// AssignGroup moves the user to a group of the user's organisation (the
// organisation stored on the user). The check and the update are one
// statement, so they cannot race. It returns domain.ErrUserNotFound,
// domain.ErrGroupNotFound, or domain.ErrGroupNotInOrganisation when the
// update does not happen.
func (r *UserRepository) AssignGroup(ctx context.Context, userID, groupID int64) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE users u SET group_id = g.id, updated_at = NOW()
		 FROM groups g
		 WHERE u.id = $2 AND g.id = $1 AND g.organisation_id = u.organisation_id`,
		groupID, userID)
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
	var userOrg, groupOrg sql.NullInt64
	err = r.db.QueryRowContext(ctx,
		`SELECT (SELECT organisation_id FROM users WHERE id = $1),
		        (SELECT organisation_id FROM groups WHERE id = $2)`,
		userID, groupID).Scan(&userOrg, &groupOrg)
	if err != nil {
		return err
	}

	switch {
	case !userOrg.Valid:
		return domain.ErrUserNotFound
	case !groupOrg.Valid:
		return domain.ErrGroupNotFound
	default:
		return domain.ErrGroupNotInOrganisation
	}
}
