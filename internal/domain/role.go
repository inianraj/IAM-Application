package domain

import "fmt"

// Every organisation has these four roles when it is created. They cannot be
// renamed, changed or deleted. owner has every permission; admin the same
// except deleting the organisation; member and guest are the basic sets
// (MemberPermissions, GuestPermissions).
const (
	OwnerRoleName  = "owner"
	AdminRoleName  = "admin"
	MemberRoleName = "member"
	GuestRoleName  = "guest"
)

// Role is a named set of permissions, held by users directly or through their
// group.
type Role struct {
	ID             int64  `json:"id"`
	OrganisationID int64  `json:"organisation_id"`
	Name           string `json:"name"`
}

// Validate normalizes the name like a group name and checks the organisation.
func (r *Role) Validate() error {
	if r.OrganisationID <= 0 {
		return ErrInvalidOrganisationID
	}

	name, err := NormalizeGroupName(r.Name)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidRole, err.Error())
	}
	r.Name = name

	return nil
}

// IsBuiltinRole reports whether name is one of the four roles every
// organisation gets.
func IsBuiltinRole(name string) bool {
	switch name {
	case OwnerRoleName, AdminRoleName, MemberRoleName, GuestRoleName:
		return true
	}
	return false
}
