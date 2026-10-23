package service

import (
	"context"

	"crud/internal/domain"
)

type GroupStore interface {
	Create(ctx context.Context, g domain.Group) (domain.Group, error)
	GetByID(ctx context.Context, id int64) (domain.Group, error)
	ListByOrganisation(ctx context.Context, organisationID int64) ([]domain.Group, error)
	Update(ctx context.Context, g domain.Group) (domain.Group, error)
	Delete(ctx context.Context, id int64) error
	ListMembers(ctx context.Context, groupID int64) ([]domain.GroupMember, error)
}

type GroupService struct {
	store GroupStore
}

func NewGroupService(store GroupStore) *GroupService {
	return &GroupService{store: store}
}

func checkGroupID(id int64) error {
	if id <= 0 {
		return domain.ErrInvalidGroupID
	}
	return nil
}

func (s *GroupService) Create(ctx context.Context, g domain.Group) (domain.Group, error) {
	if err := g.Validate(); err != nil {
		return domain.Group{}, err
	}
	return s.store.Create(ctx, g)
}

func (s *GroupService) Get(ctx context.Context, id int64) (domain.Group, error) {
	if err := checkGroupID(id); err != nil {
		return domain.Group{}, err
	}
	return s.store.GetByID(ctx, id)
}

func (s *GroupService) ListByOrganisation(ctx context.Context, organisationID int64) ([]domain.Group, error) {
	if organisationID <= 0 {
		return nil, domain.ErrInvalidOrganisationID
	}
	return s.store.ListByOrganisation(ctx, organisationID)
}

func (s *GroupService) Update(ctx context.Context, g domain.Group) (domain.Group, error) {
	if err := checkGroupID(g.ID); err != nil {
		return domain.Group{}, err
	}
	if err := g.Validate(); err != nil {
		return domain.Group{}, err
	}
	return s.store.Update(ctx, g)
}

func (s *GroupService) Delete(ctx context.Context, id int64) error {
	if err := checkGroupID(id); err != nil {
		return err
	}
	return s.store.Delete(ctx, id)
}

func (s *GroupService) ListMembers(ctx context.Context, groupID int64) ([]domain.GroupMember, error) {
	if err := checkGroupID(groupID); err != nil {
		return nil, err
	}
	return s.store.ListMembers(ctx, groupID)
}
