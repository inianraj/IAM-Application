package domain

import "errors"

var (
	ErrTaskNotFound  = errors.New("record not found")
	ErrInvalidTask   = errors.New("invalid record")
	ErrInvalidTaskID = errors.New("invalid record ID")
	ErrTaskConflict  = errors.New("record conflict")

	ErrInvalidUser    = errors.New("invalid user")
	ErrInvalidUserID  = errors.New("invalid user ID")
	ErrUserNotFound   = errors.New("user not found")
	ErrUserInUse      = errors.New("user still has records")
	ErrEmailTaken     = errors.New("email already registered")
	ErrInvalidGroup   = errors.New("invalid group")
	ErrInvalidGroupID = errors.New("invalid group ID")
	ErrGroupNotFound  = errors.New("group not found")
	ErrGroupExists    = errors.New("group already exists")
	ErrGroupInUse     = errors.New("group still has users")

	ErrGroupNotInOrganisation = errors.New("group does not belong to the user's organisation")

	ErrInvalidRole   = errors.New("invalid role")
	ErrInvalidRoleID = errors.New("invalid role ID")
	ErrRoleNotFound  = errors.New("role not found")
	ErrRoleExists    = errors.New("role already exists")
	ErrRoleInUse     = errors.New("role is still assigned to users or groups")
	ErrBuiltinRole   = errors.New("built-in roles (owner, admin, member, guest) cannot be renamed, changed or deleted")
	ErrLastOwner     = errors.New("an organisation must keep at least one owner")

	ErrInvalidOrganisation   = errors.New("invalid organisation")
	ErrInvalidOrganisationID = errors.New("invalid organisation ID")
	ErrOrganisationNotFound  = errors.New("organisation not found")
	ErrOrganisationExists    = errors.New("organisation already exists")
	ErrOrganisationInUse     = errors.New("organisation still has groups with users")

	// Authentication and authorization.
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrWrongSignupMethod  = errors.New("wrong sign-in method")
	ErrTooManyAttempts    = errors.New("too many failed login attempts, try again later")
	ErrUnauthenticated    = errors.New("authentication required")
	ErrForbidden          = errors.New("forbidden")
	ErrUnavailable        = errors.New("service temporarily unavailable")
)
