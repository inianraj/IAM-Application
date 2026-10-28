package service

import (
	"context"

	"crud/internal/authz"
	"crud/internal/domain"
)

type TaskStore interface {
	Create(ctx context.Context, t domain.Task) (domain.Task, error)
	GetByID(ctx context.Context, id int64) (domain.Task, error)
	ListByUser(ctx context.Context, userID int64) ([]domain.Task, error)
	ListByOrganisation(ctx context.Context, organisationID int64) ([]domain.Task, error)
	Update(ctx context.Context, t domain.Task) (domain.Task, error)
	Delete(ctx context.Context, id int64) error
}

// TaskService applies the record rules. A record belongs to one user and to
// that user's organisation; what the caller may do with it comes from the
// record:* permissions of their group (":own" for their own records, ":org"
// for any record of their organisation).
type TaskService struct {
	store TaskStore
	authz *authz.Authorizer
}

func NewTaskService(store TaskStore, az *authz.Authorizer) *TaskService {
	return &TaskService{store: store, authz: az}
}

func checkID(id int64) error {
	if id <= 0 {
		return domain.ErrInvalidTaskID
	}
	return nil
}

func (s *TaskService) Create(ctx context.Context, t domain.Task) (domain.Task, error) {
	if t.UserID <= 0 {
		return domain.Task{}, domain.ErrInvalidUserID
	}
	owner, err := s.authz.RequireScopedForUser(ctx, domain.ResourceRecord, domain.ActionCreate, t.UserID, domain.ErrUserNotFound)
	if err != nil {
		return domain.Task{}, err
	}
	if err := t.Validate(); err != nil {
		return domain.Task{}, err
	}

	t.OrganisationID = owner.OrganisationID
	return s.store.Create(ctx, t)
}

func (s *TaskService) Get(ctx context.Context, id int64) (domain.Task, error) {
	if err := checkID(id); err != nil {
		return domain.Task{}, err
	}
	return s.allowed(ctx, id, domain.ActionRead)
}

// ListByUser returns one user's records.
func (s *TaskService) ListByUser(ctx context.Context, userID int64) ([]domain.Task, error) {
	if userID <= 0 {
		return nil, domain.ErrInvalidUserID
	}
	if _, err := s.authz.RequireScopedForUser(ctx, domain.ResourceRecord, domain.ActionRead, userID, domain.ErrUserNotFound); err != nil {
		return nil, err
	}
	return s.store.ListByUser(ctx, userID)
}

// ListMine returns the caller's own records.
func (s *TaskService) ListMine(ctx context.Context) ([]domain.Task, error) {
	caller, err := s.authz.Caller(ctx)
	if err != nil {
		return nil, err
	}
	return s.ListByUser(ctx, caller.ID)
}

// ListByOrganisation returns every record of the caller's organisation. It
// needs the record:read:org permission.
func (s *TaskService) ListByOrganisation(ctx context.Context) ([]domain.Task, error) {
	caller, err := s.authz.Caller(ctx)
	if err != nil {
		return nil, err
	}
	perm := domain.ScopedKey(domain.ResourceRecord, domain.ActionRead, domain.ScopeOrg)
	if err := s.authz.Require(ctx, perm, caller.OrganisationID, domain.ErrOrganisationNotFound); err != nil {
		return nil, err
	}
	return s.store.ListByOrganisation(ctx, caller.OrganisationID)
}

func (s *TaskService) Update(ctx context.Context, t domain.Task) (domain.Task, error) {
	if err := checkID(t.ID); err != nil {
		return domain.Task{}, err
	}
	if _, err := s.allowed(ctx, t.ID, domain.ActionUpdate); err != nil {
		return domain.Task{}, err
	}
	if err := t.Validate(); err != nil {
		return domain.Task{}, err
	}
	return s.store.Update(ctx, t)
}

func (s *TaskService) Delete(ctx context.Context, id int64) error {
	if err := checkID(id); err != nil {
		return err
	}
	if _, err := s.allowed(ctx, id, domain.ActionDelete); err != nil {
		return err
	}
	return s.store.Delete(ctx, id)
}

// allowed loads a record and checks that the caller may do action on it,
// using the record's own owner and organisation.
func (s *TaskService) allowed(ctx context.Context, id int64, action string) (domain.Task, error) {
	existing, err := s.store.GetByID(ctx, id)
	if err != nil {
		return domain.Task{}, err
	}
	if err := s.authz.RequireScoped(ctx, domain.ResourceRecord, action, existing.UserID, existing.OrganisationID, domain.ErrTaskNotFound); err != nil {
		return domain.Task{}, err
	}
	return existing, nil
}
