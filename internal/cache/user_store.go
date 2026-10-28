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
	Create(ctx context.Context, u domain.User) (domain.User, error)
	GetByID(ctx context.Context, id int64) (domain.User, error)
	Update(ctx context.Context, u domain.User) (domain.User, error)
	Delete(ctx context.Context, id int64) error
	AssignGroup(ctx context.Context, userID, groupID int64) error
}

// CachedUserStore is a cache-aside decorator over a UserStore. Only
// GetByID is cached; every write that changes the user drops its key.
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

// Create only drops the member list of the group the user joined; the user's
// own key can't exist yet.
func (s *CachedUserStore) Create(ctx context.Context, u domain.User) (domain.User, error) {
	created, err := s.next.Create(ctx, u)
	if err != nil {
		return domain.User{}, err
	}
	s.invalidate(ctx, groupMembersKey(created.GroupID))
	return created, nil
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
	s.invalidate(ctx, userKey(id), groupMembersKey(existing.GroupID))
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
	s.invalidate(ctx, userKey(userID), groupMembersKey(before.GroupID), groupMembersKey(groupID))
	return nil
}
