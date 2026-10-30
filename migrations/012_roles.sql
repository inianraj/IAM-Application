-- Roles: permissions are bundled into roles, and a user gets the union of the
-- permissions of the roles they hold directly (user_roles) and of the roles of
-- their group (group_roles):  permissions -> roles -> users / groups -> users.
-- Roles belong to an organisation. A group is optional now: users.group_id may
-- be NULL while the user still has an organisation.
--
-- Every organisation has four built-in roles that cannot be edited or deleted:
-- owner (every permission), admin (every permission except
-- organisation:delete), member (the basic set) and guest (read your own
-- records and see the organisation). A new organisation's creator gets owner;
-- users added later get member unless told otherwise.
--
-- Existing data keeps its access: every group gets a role with the same name
-- and permissions, linked to it (the old admin and member groups are ordinary
-- groups now, linked to the built-in admin and member roles). In each
-- organisation the lowest-id user of the admin group also gets the owner role.
-- group_permissions is dropped, and group:manage_permissions is replaced by
-- role:create, role:read, role:update, role:delete and role:assign.
BEGIN;

CREATE TABLE IF NOT EXISTS roles (
    id              BIGSERIAL PRIMARY KEY,
    organisation_id BIGINT      NOT NULL REFERENCES organisations(id) ON DELETE CASCADE,
    name            VARCHAR(50) NOT NULL,
    CONSTRAINT roles_organisation_id_name_key UNIQUE (organisation_id, name),
    CONSTRAINT roles_id_organisation_id_key   UNIQUE (id, organisation_id)
);

CREATE TABLE IF NOT EXISTS role_permissions (
    role_id        BIGINT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_key TEXT   NOT NULL REFERENCES permissions(key) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_key)
);

-- The composite foreign keys below prove that a group (or user) and its role
-- are in the same organisation; they use the unique (id, organisation_id)
-- indexes of groups (migration 005) and users (migration 011).
--
-- Deleting a role that a group or user still has is refused (no ON DELETE
-- action on the role side); deleting the group, the user or the organisation
-- removes the links.
CREATE TABLE IF NOT EXISTS group_roles (
    group_id        BIGINT NOT NULL,
    role_id         BIGINT NOT NULL,
    organisation_id BIGINT NOT NULL,
    PRIMARY KEY (group_id, role_id),
    CONSTRAINT group_roles_group_fkey FOREIGN KEY (group_id, organisation_id)
        REFERENCES groups (id, organisation_id) ON DELETE CASCADE,
    CONSTRAINT group_roles_role_fkey FOREIGN KEY (role_id, organisation_id)
        REFERENCES roles (id, organisation_id)
);

CREATE INDEX IF NOT EXISTS group_roles_role_id_idx ON group_roles (role_id);

CREATE TABLE IF NOT EXISTS user_roles (
    user_id         BIGINT NOT NULL,
    role_id         BIGINT NOT NULL,
    organisation_id BIGINT NOT NULL,
    PRIMARY KEY (user_id, role_id),
    CONSTRAINT user_roles_user_fkey FOREIGN KEY (user_id, organisation_id)
        REFERENCES users (id, organisation_id) ON DELETE CASCADE,
    CONSTRAINT user_roles_role_fkey FOREIGN KEY (role_id, organisation_id)
        REFERENCES roles (id, organisation_id)
);

CREATE INDEX IF NOT EXISTS user_roles_role_id_idx ON user_roles (role_id);

-- A group is optional, but a group still needs an organisation.
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_org_group_both_or_neither;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_group_needs_organisation;
ALTER TABLE users
    ADD CONSTRAINT users_group_needs_organisation
    CHECK (group_id IS NULL OR organisation_id IS NOT NULL);

-- 'owner' and 'guest' are built-in role names: move a custom group that
-- already has one so its role does not take the built-in's place.
UPDATE groups SET name = name || '-legacy' WHERE name IN ('owner', 'guest');

-- One role per group, with the group's name and permissions, plus the four
-- built-in roles in every organisation.
INSERT INTO roles (organisation_id, name)
SELECT organisation_id, name FROM groups
ON CONFLICT DO NOTHING;

INSERT INTO roles (organisation_id, name)
SELECT o.id, n.name
FROM organisations o CROSS JOIN (VALUES ('owner'), ('admin'), ('member'), ('guest')) AS n(name)
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_key)
SELECT r.id, gp.permission
FROM group_permissions gp
JOIN groups g ON g.id = gp.group_id
JOIN roles r  ON r.organisation_id = g.organisation_id AND r.name = g.name
ON CONFLICT DO NOTHING;

INSERT INTO group_roles (group_id, role_id, organisation_id)
SELECT g.id, r.id, g.organisation_id
FROM groups g
JOIN roles r ON r.organisation_id = g.organisation_id AND r.name = g.name
ON CONFLICT DO NOTHING;

-- group:manage_permissions is replaced by the role permissions.
DELETE FROM permissions WHERE key = 'group:manage_permissions';

INSERT INTO permissions (key, resource, action, scope, description) VALUES
    ('role:create', 'role', 'create', NULL, 'Create roles, with the permissions they hold'),
    ('role:read',   'role', 'read',   NULL, 'See the roles, what they allow and the permission list'),
    ('role:update', 'role', 'update', NULL, 'Rename roles and change what they allow'),
    ('role:delete', 'role', 'delete', NULL, 'Delete roles that no user or group has'),
    ('role:assign', 'role', 'assign', NULL, 'Choose which roles users and groups have')
ON CONFLICT (key) DO NOTHING;

-- The built-in roles get exactly their permissions.
DELETE FROM role_permissions
WHERE role_id IN (SELECT id FROM roles WHERE name IN ('owner', 'admin', 'member', 'guest'));

INSERT INTO role_permissions (role_id, permission_key)
SELECT r.id, p.key
FROM roles r CROSS JOIN permissions p
WHERE r.name = 'owner'
   OR (r.name = 'admin'  AND p.key <> 'organisation:delete')
   OR (r.name = 'member' AND p.key IN ('record:create:own', 'record:read:own', 'record:update:own',
                                       'record:delete:own', 'user:read', 'user:update:own',
                                       'group:read', 'organisation:read'))
   OR (r.name = 'guest'  AND p.key IN ('record:read:own', 'organisation:read'));

-- The first admin of each organisation becomes its owner.
INSERT INTO user_roles (user_id, role_id, organisation_id)
SELECT f.user_id, r.id, f.organisation_id
FROM (
    SELECT DISTINCT ON (u.organisation_id) u.id AS user_id, u.organisation_id
    FROM users u
    JOIN groups g ON g.id = u.group_id AND g.name = 'admin'
    ORDER BY u.organisation_id, u.id
) f
JOIN roles r ON r.organisation_id = f.organisation_id AND r.name = 'owner'
ON CONFLICT DO NOTHING;

DROP TABLE group_permissions;

COMMIT;
