package cache

import (
	"context"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"crud/internal/domain"
)

// TaskStore is the persistence contract the cache wraps (same shape as service.TaskStore).
type TaskStore interface {
	Create(ctx context.Context, t domain.Task) (domain.Task, error)
	GetByID(ctx context.Context, id int64) (domain.Task, error)
	ListByUser(ctx context.Context, userID int64) ([]domain.Task, error)
	ListByOrganisation(ctx context.Context, organisationID int64) ([]domain.Task, error)
	Update(ctx context.Context, t domain.Task) (domain.Task, error)
	Delete(ctx context.Context, id int64) error
}

// CachedTaskStore is a cache-aside decorator over a TaskStore.
type CachedTaskStore struct {
	jsonCache
	next TaskStore
}

func NewTaskStore(next TaskStore, rdb *redis.Client, ttl time.Duration, logger *slog.Logger) *CachedTaskStore {
	return &CachedTaskStore{jsonCache: jsonCache{rdb: rdb, ttl: ttl, logger: logger}, next: next}
}

func (s *CachedTaskStore) GetByID(ctx context.Context, id int64) (domain.Task, error) {
	var t domain.Task
	if s.get(ctx, recordKey(id), &t) {
		return t, nil
	}

	t, err := s.next.GetByID(ctx, id)
	if err != nil {
		return domain.Task{}, err
	}
	s.set(ctx, recordKey(id), t)
	return t, nil
}

func (s *CachedTaskStore) ListByUser(ctx context.Context, userID int64) ([]domain.Task, error) {
	var list []domain.Task
	if s.get(ctx, userRecordsKey(userID), &list) && list != nil {
		return list, nil
	}

	list, err := s.next.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	s.set(ctx, userRecordsKey(userID), list)
	return list, nil
}

func (s *CachedTaskStore) ListByOrganisation(ctx context.Context, organisationID int64) ([]domain.Task, error) {
	var list []domain.Task
	if s.get(ctx, orgRecordsKey(organisationID), &list) && list != nil {
		return list, nil
	}

	list, err := s.next.ListByOrganisation(ctx, organisationID)
	if err != nil {
		return nil, err
	}
	s.set(ctx, orgRecordsKey(organisationID), list)
	return list, nil
}

func (s *CachedTaskStore) Create(ctx context.Context, t domain.Task) (domain.Task, error) {
	created, err := s.next.Create(ctx, t)
	if err != nil {
		return domain.Task{}, err
	}
	s.invalidate(ctx, userRecordsKey(created.UserID), orgRecordsKey(created.OrganisationID))
	return created, nil
}

func (s *CachedTaskStore) Update(ctx context.Context, t domain.Task) (domain.Task, error) {
	updated, err := s.next.Update(ctx, t)
	if err != nil {
		return domain.Task{}, err
	}
	s.invalidate(ctx, recordKey(updated.ID), userRecordsKey(updated.UserID), orgRecordsKey(updated.OrganisationID))
	return updated, nil
}

func (s *CachedTaskStore) Delete(ctx context.Context, id int64) error {
	// Read the owner from Postgres (not the cache) so the list key is right.
	existing, err := s.next.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.next.Delete(ctx, id); err != nil {
		return err
	}
	s.invalidate(ctx, recordKey(id), userRecordsKey(existing.UserID), orgRecordsKey(existing.OrganisationID))
	return nil
}
