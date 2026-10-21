package domain

import (
	"fmt"
	"strings"
)

const (
	maxTitleLength = 200
)

// Validate trims and checks the task fields.
func (t *Task) Validate() error {
	t.Title = strings.TrimSpace(t.Title)
	t.Description = strings.TrimSpace(t.Description)

	if t.Title == "" {
		return fmt.Errorf("%w: title is required", ErrInvalidTask)
	}

	if len(t.Title) > maxTitleLength {
		return fmt.Errorf("%w: title must be at most %d characters", ErrInvalidTask, maxTitleLength)
	}

	return nil
}
