package domain

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const maxOrganisationNameLength = 100

type Organisation struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Validate trims and checks the organisation fields.
func (o *Organisation) Validate() error {
	o.Name = strings.TrimSpace(o.Name)

	if o.Name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidOrganisation)
	}

	if utf8.RuneCountInString(o.Name) > maxOrganisationNameLength {
		return fmt.Errorf("%w: name must be at most %d characters", ErrInvalidOrganisation, maxOrganisationNameLength)
	}

	return nil
}

// DeletedOrganisation lists what deleting an organisation removed besides the
// organisation and its groups, so cached copies can be dropped.
type DeletedOrganisation struct {
	UserIDs   []int64
	RecordIDs []int64
}
