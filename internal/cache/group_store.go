package cache

import (
	"context"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"crud/internal/domain"
)

// GroupStore is the persistence contract the cache wraps (same shape as service.GroupStore).
type GroupStore interface {
	Create(ctx context.Context, g domain.Group, roleIDs []int64) (domain.Group, error)
	GetByID(ctx context.Context, id int64) (domain.Group, error)
	ListByOrganisation(ctx context.Context, organisationID int64) ([]domain.Group, error)
	Update(ctx context.Context, g domain.Group) (domain.Group, error)
	Delete(ctx context.Context, id int64) error
	ListMembers(ctx context.Context, groupID int64) ([]domain.GroupMember, error)

	Roles(ctx context.Context, groupID int64) ([]domain.Role, error)
	SetRoles(ctx context.Context, groupID int64, roleIDs []int64) error
}

// CachedGroupStore caches GetByID, the per-organisation list and the member
// list of a group. User writes that change a member list are invalidated in
// CachedUserStore. A change to a group's roles drops the cached permissions of
// its members.
type CachedGroupStore struct {
	jsonCache
	next GroupStore
}

func NewGroupStore(next GroupStore, rdb *redis.Client, ttl time.Duration, logger *slog.Logger) *CachedGroupStore {
	return &CachedGroupStore{jsonCache: jsonCache{rdb: rdb, ttl: ttl, logger: logger}, next: next}
}

func (s *CachedGroupStore) GetByID(ctx context.Context, id int64) (domain.Group, error) {
	var g domain.Group
	if s.get(ctx, groupKey(id), &g) {
		return g, nil
	}

	g, err := s.next.GetByID(ctx, id)
	if err != nil {
		return domain.Group{}, err
	}
	s.set(ctx, groupKey(id), g)
	return g, nil
}

func (s *CachedGroupStore) ListByOrganisation(ctx context.Context, organisationID int64) ([]domain.Group, error) {
	var list []domain.Group
	if s.get(ctx, orgGroupsKey(organisationID), &list) && list != nil {
		return list, nil
	}

	list, err := s.next.ListByOrganisation(ctx, organisationID)
	if err != nil {
		return nil, err
	}
	s.set(ctx, orgGroupsKey(organisationID), list)
	return list, nil
}

func (s *CachedGroupStore) ListMembers(ctx context.Context, groupID int64) ([]domain.GroupMember, error) {
	var members []domain.GroupMember
	if s.get(ctx, groupMembersKey(groupID), &members) && members != nil {
		return members, nil
	}

	members, err := s.next.ListMembers(ctx, groupID)
	if err != nil {
		return nil, err
	}
	s.set(ctx, groupMembersKey(groupID), members)
	return members, nil
}

func (s *CachedGroupStore) Create(ctx context.Context, g domain.Group, roleIDs []int64) (domain.Group, error) {
	created, err := s.next.Create(ctx, g, roleIDs)
	if err != nil {
		return domain.Group{}, err
	}
	s.invalidate(ctx, orgGroupsKey(created.OrganisationID))
	return created, nil
}

func (s *CachedGroupStore) Update(ctx context.Context, g domain.Group) (domain.Group, error) {
	// The group may move to another organisation, so read its current one
	// from Postgres (not the cache) to invalidate that organisation's list too.
	before, err := s.next.GetByID(ctx, g.ID)
	if err != nil {
		return domain.Group{}, err
	}

	updated, err := s.next.Update(ctx, g)
	if err != nil {
		return domain.Group{}, err
	}
	s.invalidate(ctx, groupKey(updated.ID), orgGroupsKey(before.OrganisationID), orgGroupsKey(updated.OrganisationID))
	return updated, nil
}

func (s *CachedGroupStore) Delete(ctx context.Context, id int64) error {
	existing, err := s.next.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.next.Delete(ctx, id); err != nil {
		return err
	}
	s.invalidate(ctx, groupKey(id), groupMembersKey(id), orgGroupsKey(existing.OrganisationID))
	return nil
}

func (s *CachedGroupStore) Roles(ctx context.Context, groupID int64) ([]domain.Role, error) {
	return s.next.Roles(ctx, groupID)
}

// SetRoles changes what every member of the group may do, so their cached
// permissions are dropped. The members are listed from Postgres (not the
// cache) after the change; if that fails the TTL is the backstop.
func (s *CachedGroupStore) SetRoles(ctx context.Context, groupID int64, roleIDs []int64) error {
	if err := s.next.SetRoles(ctx, groupID, roleIDs); err != nil {
		return err
	}

	members, err := s.next.ListMembers(ctx, groupID)
	if err != nil {
		s.logger.Warn("listing group members for invalidation failed; relying on TTL", "group_id", groupID, "error", err)
		return nil
	}
	keys := make([]string, 0, len(members))
	for _, m := range members {
		keys = append(keys, userPermissionsKey(m.ID))
	}
	if len(keys) > 0 {
		s.invalidate(ctx, keys...)
	}
	return nil
}
