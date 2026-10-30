package cache

import (
	"context"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"crud/internal/domain"
)

// UserStore is the persistence contract the cache wraps (same shape as service.UserStore).
type UserStore interface {
	Create(ctx context.Context, u domain.User, roleIDs []int64) (domain.User, error)
	GetByID(ctx context.Context, id int64) (domain.User, error)
	ListByOrganisation(ctx context.Context, organisationID int64) ([]domain.User, error)
	Update(ctx context.Context, u domain.User) (domain.User, error)
	Delete(ctx context.Context, id int64) error
	AssignGroup(ctx context.Context, userID, groupID int64) error

	Permissions(ctx context.Context, userID int64) ([]string, error)
	Roles(ctx context.Context, userID int64) ([]domain.Role, error)
	SetRoles(ctx context.Context, userID int64, roleIDs []int64) error
}

// CachedUserStore is a cache-aside decorator over a UserStore. GetByID and the
// user's permissions (what authorization reads on every request) are cached;
// every write that changes the user drops their keys.
type CachedUserStore struct {
	jsonCache
	next UserStore
}

func NewUserStore(next UserStore, rdb *redis.Client, ttl time.Duration, logger *slog.Logger) *CachedUserStore {
	return &CachedUserStore{jsonCache: jsonCache{rdb: rdb, ttl: ttl, logger: logger}, next: next}
}

func (s *CachedUserStore) GetByID(ctx context.Context, id int64) (domain.User, error) {
	var u domain.User
	if s.get(ctx, userKey(id), &u) {
		return u, nil
	}

	u, err := s.next.GetByID(ctx, id)
	if err != nil {
		return domain.User{}, err
	}
	s.set(ctx, userKey(id), u)
	return u, nil
}

// Create has nothing to drop: a new user has no group, and none of their keys
// can exist yet.
func (s *CachedUserStore) Create(ctx context.Context, u domain.User, roleIDs []int64) (domain.User, error) {
	return s.next.Create(ctx, u, roleIDs)
}

func (s *CachedUserStore) Update(ctx context.Context, u domain.User) (domain.User, error) {
	updated, err := s.next.Update(ctx, u)
	if err != nil {
		return domain.User{}, err
	}
	// Name and email appear in the group's member list.
	s.invalidate(ctx, userKey(updated.ID), groupMembersKey(updated.GroupID))
	return updated, nil
}

func (s *CachedUserStore) Delete(ctx context.Context, id int64) error {
	// Read the group from Postgres (not the cache) so its member list is right.
	existing, err := s.next.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.next.Delete(ctx, id); err != nil {
		return err
	}
	s.invalidate(ctx, userKey(id), userPermissionsKey(id), groupMembersKey(existing.GroupID))
	return nil
}

func (s *CachedUserStore) AssignGroup(ctx context.Context, userID, groupID int64) error {
	before, err := s.next.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if err := s.next.AssignGroup(ctx, userID, groupID); err != nil {
		return err
	}
	// The group's roles change what the user may do.
	s.invalidate(ctx, userKey(userID), userPermissionsKey(userID), groupMembersKey(before.GroupID), groupMembersKey(groupID))
	return nil
}

// Permissions is what authorization reads on every request, so it is cached.
func (s *CachedUserStore) Permissions(ctx context.Context, userID int64) ([]string, error) {
	var perms []string
	if s.get(ctx, userPermissionsKey(userID), &perms) && perms != nil {
		return perms, nil
	}

	perms, err := s.next.Permissions(ctx, userID)
	if err != nil {
		return nil, err
	}
	s.set(ctx, userPermissionsKey(userID), perms)
	return perms, nil
}

func (s *CachedUserStore) Roles(ctx context.Context, userID int64) ([]domain.Role, error) {
	return s.next.Roles(ctx, userID)
}

// SetRoles changes what the user may do, so their cached permissions go.
func (s *CachedUserStore) SetRoles(ctx context.Context, userID int64, roleIDs []int64) error {
	if err := s.next.SetRoles(ctx, userID, roleIDs); err != nil {
		return err
	}
	s.invalidate(ctx, userPermissionsKey(userID))
	return nil
}

// ListByOrganisation is read rarely and changes with every user write, so it is
// not cached.
func (s *CachedUserStore) ListByOrganisation(ctx context.Context, organisationID int64) ([]domain.User, error) {
	return s.next.ListByOrganisation(ctx, organisationID)
}
