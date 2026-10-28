package cache

import (
	"context"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"crud/internal/domain"
)

// OrganisationStore is the persistence contract the cache wraps (same shape as service.OrganisationStore).
type OrganisationStore interface {
	Create(ctx context.Context, o domain.Organisation, admin domain.User) (domain.Organisation, domain.User, error)
	GetByID(ctx context.Context, id int64) (domain.Organisation, error)
	List(ctx context.Context) ([]domain.Organisation, error)
	Update(ctx context.Context, o domain.Organisation) (domain.Organisation, error)
	Delete(ctx context.Context, id int64) (domain.DeletedOrganisation, error)
}

// GroupLister lists an organisation's groups straight from the database; it
// lets Delete drop the cache entries of the groups the cascade removes.
type GroupLister interface {
	ListByOrganisation(ctx context.Context, organisationID int64) ([]domain.Group, error)
}

// CachedOrganisationStore caches GetByID and the full list.
type CachedOrganisationStore struct {
	jsonCache
	next   OrganisationStore
	groups GroupLister
}

func NewOrganisationStore(next OrganisationStore, groups GroupLister, rdb *redis.Client, ttl time.Duration, logger *slog.Logger) *CachedOrganisationStore {
	return &CachedOrganisationStore{jsonCache: jsonCache{rdb: rdb, ttl: ttl, logger: logger}, next: next, groups: groups}
}

func (s *CachedOrganisationStore) GetByID(ctx context.Context, id int64) (domain.Organisation, error) {
	var o domain.Organisation
	if s.get(ctx, organisationKey(id), &o) {
		return o, nil
	}

	o, err := s.next.GetByID(ctx, id)
	if err != nil {
		return domain.Organisation{}, err
	}
	s.set(ctx, organisationKey(id), o)
	return o, nil
}

func (s *CachedOrganisationStore) List(ctx context.Context) ([]domain.Organisation, error) {
	var list []domain.Organisation
	if s.get(ctx, organisationsKey, &list) && list != nil {
		return list, nil
	}

	list, err := s.next.List(ctx)
	if err != nil {
		return nil, err
	}
	s.set(ctx, organisationsKey, list)
	return list, nil
}

// Create also creates the admin user and the organisation's groups; none of
// them can be cached yet, so only the lists that would now be stale are dropped.
func (s *CachedOrganisationStore) Create(ctx context.Context, o domain.Organisation, admin domain.User) (domain.Organisation, domain.User, error) {
	created, createdAdmin, err := s.next.Create(ctx, o, admin)
	if err != nil {
		return domain.Organisation{}, domain.User{}, err
	}
	s.invalidate(ctx, organisationsKey, orgGroupsKey(created.ID))
	return created, createdAdmin, nil
}

func (s *CachedOrganisationStore) Update(ctx context.Context, o domain.Organisation) (domain.Organisation, error) {
	updated, err := s.next.Update(ctx, o)
	if err != nil {
		return domain.Organisation{}, err
	}
	s.invalidate(ctx, organisationKey(updated.ID), organisationsKey)
	return updated, nil
}

func (s *CachedOrganisationStore) Delete(ctx context.Context, id int64) (domain.DeletedOrganisation, error) {
	// The delete cascades to the organisation's groups, so collect them first.
	// A lookup failure is not fatal: the TTL covers the group keys.
	groups, listErr := s.groups.ListByOrganisation(ctx, id)

	deleted, err := s.next.Delete(ctx, id)
	if err != nil {
		return domain.DeletedOrganisation{}, err
	}

	keys := []string{organisationKey(id), organisationsKey, orgGroupsKey(id), orgRecordsKey(id)}
	if listErr == nil {
		for _, g := range groups {
			keys = append(keys, groupKey(g.ID), groupMembersKey(g.ID), groupPermissionsKey(g.ID))
		}
	}
	// The users and their records went with the organisation.
	for _, uid := range deleted.UserIDs {
		keys = append(keys, userKey(uid), userRecordsKey(uid))
	}
	for _, rid := range deleted.RecordIDs {
		keys = append(keys, recordKey(rid))
	}
	s.invalidate(ctx, keys...)
	return deleted, nil
}
