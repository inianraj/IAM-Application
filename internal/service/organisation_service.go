package service

import (
	"context"

	"crud/internal/domain"
)

type OrganisationStore interface {
	Create(ctx context.Context, o domain.Organisation) (domain.Organisation, error)
	GetByID(ctx context.Context, id int64) (domain.Organisation, error)
	List(ctx context.Context) ([]domain.Organisation, error)
	Update(ctx context.Context, o domain.Organisation) (domain.Organisation, error)
	Delete(ctx context.Context, id int64) error
}

type OrganisationService struct {
	store OrganisationStore
}

func NewOrganisationService(store OrganisationStore) *OrganisationService {
	return &OrganisationService{store: store}
}

func checkOrganisationID(id int64) error {
	if id <= 0 {
		return domain.ErrInvalidOrganisationID
	}
	return nil
}

func (s *OrganisationService) Create(ctx context.Context, o domain.Organisation) (domain.Organisation, error) {
	if err := o.Validate(); err != nil {
		return domain.Organisation{}, err
	}
	return s.store.Create(ctx, o)
}

func (s *OrganisationService) Get(ctx context.Context, id int64) (domain.Organisation, error) {
	if err := checkOrganisationID(id); err != nil {
		return domain.Organisation{}, err
	}
	return s.store.GetByID(ctx, id)
}

func (s *OrganisationService) List(ctx context.Context) ([]domain.Organisation, error) {
	return s.store.List(ctx)
}

func (s *OrganisationService) Update(ctx context.Context, o domain.Organisation) (domain.Organisation, error) {
	if err := checkOrganisationID(o.ID); err != nil {
		return domain.Organisation{}, err
	}
	if err := o.Validate(); err != nil {
		return domain.Organisation{}, err
	}
	return s.store.Update(ctx, o)
}

func (s *OrganisationService) Delete(ctx context.Context, id int64) error {
	if err := checkOrganisationID(id); err != nil {
		return err
	}
	return s.store.Delete(ctx, id)
}
