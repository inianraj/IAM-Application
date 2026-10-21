package domain

import "errors"

var (
	ErrTaskNotFound  = errors.New("record not found")
	ErrInvalidTask   = errors.New("invalid record")
	ErrInvalidTaskID = errors.New("invalid record ID")
	ErrTaskConflict  = errors.New("record conflict")
)
