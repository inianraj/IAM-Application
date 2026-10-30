package cache

import (
	"context"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"crud/internal/domain"
)

// RoleStore is the persistence contract the cache wraps (same shape as service.RoleStore).
type RoleStore interface {
	Create(ctx context.Context, r domain.Role, permissions []string) (domain.Role, error)
	GetByID(ctx context.Context, id int64) (domain.Role, error)
	ListByOrganisation(ctx context.Context, organisationID int64) ([]domain.Role, error)
	Update(ctx context.Context, r domain.Role) (domain.Role, error)
	Delete(ctx context.Context, id int64) error

	Permissions(ctx context.Context, roleID int64) ([]string, error)
	SetPermissions(ctx context.Context, roleID int64, permissions []string) error
	HolderIDs(ctx context.Context, roleID int64) ([]int64, error)
	ListPermissions(ctx context.Context) ([]domain.Permission, error)
}

// CachedRoleStore caches nothing itself: roles are read rarely. What it does is
// invalidate. Changing the permissions of a role changes what every user who
// has it (directly or through their group) may do, so their cached
// permissions (perms:user:{id}) are dropped. A role that is deleted has no
// holders (the database refuses otherwise), so only SetPermissions needs it.
type CachedRoleStore struct {
	jsonCache
	next RoleStore
}

func NewRoleStore(next RoleStore, rdb *redis.Client, ttl time.Duration, logger *slog.Logger) *CachedRoleStore {
	return &CachedRoleStore{jsonCache: jsonCache{rdb: rdb, ttl: ttl, logger: logger}, next: next}
}

func (s *CachedRoleStore) Create(ctx context.Context, r domain.Role, permissions []string) (domain.Role, error) {
	return s.next.Create(ctx, r, permissions)
}

func (s *CachedRoleStore) GetByID(ctx context.Context, id int64) (domain.Role, error) {
	return s.next.GetByID(ctx, id)
}

func (s *CachedRoleStore) ListByOrganisation(ctx context.Context, organisationID int64) ([]domain.Role, error) {
	return s.next.ListByOrganisation(ctx, organisationID)
}

func (s *CachedRoleStore) Update(ctx context.Context, r domain.Role) (domain.Role, error) {
	return s.next.Update(ctx, r)
}

func (s *CachedRoleStore) Delete(ctx context.Context, id int64) error {
	return s.next.Delete(ctx, id)
}

func (s *CachedRoleStore) Permissions(ctx context.Context, roleID int64) ([]string, error) {
	return s.next.Permissions(ctx, roleID)
}

func (s *CachedRoleStore) HolderIDs(ctx context.Context, roleID int64) ([]int64, error) {
	return s.next.HolderIDs(ctx, roleID)
}

func (s *CachedRoleStore) ListPermissions(ctx context.Context) ([]domain.Permission, error) {
	return s.next.ListPermissions(ctx)
}

// SetPermissions drops the cached permissions of every holder of the role. The
// holders are listed from Postgres after the change; if that fails the TTL
// is the backstop.
func (s *CachedRoleStore) SetPermissions(ctx context.Context, roleID int64, permissions []string) error {
	if err := s.next.SetPermissions(ctx, roleID, permissions); err != nil {
		return err
	}

	holders, err := s.next.HolderIDs(ctx, roleID)
	if err != nil {
		s.logger.Warn("listing role holders for invalidation failed; relying on TTL", "role_id", roleID, "error", err)
		return nil
	}
	keys := make([]string, 0, len(holders))
	for _, id := range holders {
		keys = append(keys, userPermissionsKey(id))
	}
	if len(keys) > 0 {
		s.invalidate(ctx, keys...)
	}
	return nil
}
