package domain

// A group is a set of users, such as a team. It is optional: a user may have
// none. What a group allows comes from the roles it has (see Role).
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
