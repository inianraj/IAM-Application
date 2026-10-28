package domain

// Every organisation gets these two groups when it is created. A user in the
// admin group is an admin of the organisation; the member group is the basic
// role of users created with an organisation_id.
const (
	AdminGroupName  = "admin"
	MemberGroupName = "member"
)

type Group struct {
	ID             int64  `json:"id"`
	OrganisationID int64  `json:"organisation_id"`
	Name           string `json:"name"`
}

// Validate normalizes the name (see NormalizeGroupName) and checks that the
// group belongs to an organisation.
func (g *Group) Validate() error {
	if g.OrganisationID <= 0 {
		return ErrInvalidOrganisationID
	}

	name, err := NormalizeGroupName(g.Name)
	if err != nil {
		return err
	}
	g.Name = name

	return nil
}
