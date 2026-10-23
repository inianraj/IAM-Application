package repository

import (
	"context"
	"database/sql"
	"errors"

	"crud/internal/domain"
)

type GroupRepository struct {
	db *sql.DB
}

func NewGroupRepository(db *sql.DB) *GroupRepository {
	return &GroupRepository{db: db}
}

const groupColumns = "id, organisation_id, name"

// groupOrganisationFK is the foreign key from groups to organisations.
const groupOrganisationFK = "groups_organisation_id_fkey"

// scanGroup maps sql.ErrNoRows to ErrGroupNotFound, a duplicate name within
// the organisation to ErrGroupExists, and an unknown organisation_id to
// ErrOrganisationNotFound. Any other foreign key violation is the users
// table refusing to let a group that has users move to another organisation
// (ErrGroupInUse).
func scanGroup(row interface{ Scan(...any) error }) (domain.Group, error) {
	var g domain.Group
	err := row.Scan(&g.ID, &g.OrganisationID, &g.Name)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		return g, domain.ErrGroupNotFound
	case pgCode(err) == uniqueViolation:
		return domain.Group{}, domain.ErrGroupExists
	case pgCode(err) == foreignKeyViolation && pgConstraint(err) == groupOrganisationFK:
		return domain.Group{}, domain.ErrOrganisationNotFound
	case pgCode(err) == foreignKeyViolation:
		return domain.Group{}, domain.ErrGroupInUse
	}

	return g, err
}

func (r *GroupRepository) Create(ctx context.Context, g domain.Group) (domain.Group, error) {
	return scanGroup(r.db.QueryRowContext(ctx,
		`INSERT INTO groups (organisation_id, name) VALUES ($1, $2) RETURNING `+groupColumns,
		g.OrganisationID, g.Name))
}

func (r *GroupRepository) GetByID(ctx context.Context, id int64) (domain.Group, error) {
	return scanGroup(r.db.QueryRowContext(ctx,
		`SELECT `+groupColumns+` FROM groups WHERE id = $1`, id))
}

// ListByOrganisation returns the organisation's groups ordered by id. It
// returns an empty slice (never nil) for an organisation with no groups, and
// domain.ErrOrganisationNotFound if the organisation does not exist.
func (r *GroupRepository) ListByOrganisation(ctx context.Context, organisationID int64) ([]domain.Group, error) {
	// LEFT JOIN so an organisation without groups still yields one row (with
	// NULL group columns), which tells it apart from a missing organisation.
	rows, err := r.db.QueryContext(ctx,
		`SELECT g.id, g.name
		 FROM organisations o LEFT JOIN groups g ON g.organisation_id = o.id
		 WHERE o.id = $1
		 ORDER BY g.id`,
		organisationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	found := false
	groups := []domain.Group{}
	for rows.Next() {
		found = true

		var id sql.NullInt64
		var name sql.NullString
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		if id.Valid {
			groups = append(groups, domain.Group{ID: id.Int64, OrganisationID: organisationID, Name: name.String})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if !found {
		return nil, domain.ErrOrganisationNotFound
	}

	return groups, nil
}

func (r *GroupRepository) Update(ctx context.Context, g domain.Group) (domain.Group, error) {
	updated, err := scanGroup(r.db.QueryRowContext(ctx,
		`UPDATE groups SET organisation_id = $1, name = $2 WHERE id = $3 RETURNING `+groupColumns,
		g.OrganisationID, g.Name, g.ID))
	if !errors.Is(err, domain.ErrGroupInUse) {
		return updated, err
	}

	// A group with users moved to a missing organisation breaks both foreign
	// keys, and PostgreSQL may report either. Report the missing organisation.
	var orgExists bool
	if qerr := r.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM organisations WHERE id = $1)`,
		g.OrganisationID).Scan(&orgExists); qerr != nil {
		return domain.Group{}, qerr
	}
	if !orgExists {
		return domain.Group{}, domain.ErrOrganisationNotFound
	}

	return domain.Group{}, err
}

// Delete removes the group. A group that still has users returns
// domain.ErrGroupInUse.
func (r *GroupRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM groups WHERE id = $1`, id)
	if pgCode(err) == foreignKeyViolation {
		return domain.ErrGroupInUse
	}
	if err != nil {
		return err
	}

	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrGroupNotFound
	}

	return nil
}

// ListMembers returns the users in the group, oldest first. It returns an
// empty slice (never nil) for a group with no users, and
// domain.ErrGroupNotFound if the group does not exist.
func (r *GroupRepository) ListMembers(ctx context.Context, groupID int64) ([]domain.GroupMember, error) {
	// LEFT JOIN so an empty group still yields one row (with NULL user
	// columns), which tells it apart from a missing group.
	rows, err := r.db.QueryContext(ctx,
		`SELECT u.name, u.email
		 FROM groups g LEFT JOIN users u ON u.group_id = g.id
		 WHERE g.id = $1
		 ORDER BY u.id`,
		groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	found := false
	members := []domain.GroupMember{}
	for rows.Next() {
		found = true

		var name, email sql.NullString
		if err := rows.Scan(&name, &email); err != nil {
			return nil, err
		}
		if name.Valid {
			members = append(members, domain.GroupMember{Name: name.String, Email: email.String})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if !found {
		return nil, domain.ErrGroupNotFound
	}

	return members, nil
}
