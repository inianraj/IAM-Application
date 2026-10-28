-- IAM: permissions are seeded into a table (the Go catalogue in
-- internal/domain/permission.go is the source of truth and the API keeps this
-- table in step at startup), and groups are given permissions. Everything the
-- API checks is a row here; group_permissions can only reference these rows.
--
-- Records also store the owner's organisation_id so organisation-wide
-- permissions (record:read:org ...) can be checked without loading the owner.
--
-- Existing groups keep today's behaviour: the group named 'admin' of every
-- organisation gets every permission, every other group gets the member set.
BEGIN;

CREATE TABLE IF NOT EXISTS permissions (
    key         TEXT PRIMARY KEY,
    resource    TEXT NOT NULL,
    action      TEXT NOT NULL,
    scope       TEXT,
    description TEXT NOT NULL
);

INSERT INTO permissions (key, resource, action, scope, description) VALUES
    ('record:create:own',         'record',       'create',             'own', 'Create records for yourself'),
    ('record:create:org',         'record',       'create',             'org', 'Create records for any user in the organisation'),
    ('record:read:own',           'record',       'read',               'own', 'Read your own records'),
    ('record:read:org',           'record',       'read',               'org', 'Read every record in the organisation'),
    ('record:update:own',         'record',       'update',             'own', 'Edit your own records'),
    ('record:update:org',         'record',       'update',             'org', 'Edit any record in the organisation'),
    ('record:delete:own',         'record',       'delete',             'own', 'Delete your own records'),
    ('record:delete:org',         'record',       'delete',             'org', 'Delete any record in the organisation'),
    ('user:read',                 'user',         'read',               NULL,  'Read the profile of any user in the organisation'),
    ('user:update:own',           'user',         'update',             'own', 'Change your own name'),
    ('user:update:org',           'user',         'update',             'org', 'Change the name of any user in the organisation'),
    ('user:delete',               'user',         'delete',             NULL,  'Delete users in the organisation'),
    ('user:assign_group',         'user',         'assign_group',       NULL,  'Move users between groups'),
    ('group:create',              'group',        'create',             NULL,  'Create groups'),
    ('group:read',                'group',        'read',               NULL,  'See the groups, their members and their permissions'),
    ('group:update',              'group',        'update',             NULL,  'Rename groups'),
    ('group:delete',              'group',        'delete',             NULL,  'Delete groups'),
    ('group:manage_permissions',  'group',        'manage_permissions', NULL,  'See the permission list and change what groups are allowed'),
    ('organisation:read',         'organisation', 'read',               NULL,  'See the organisation'),
    ('organisation:update',       'organisation', 'update',             NULL,  'Rename the organisation'),
    ('organisation:delete',       'organisation', 'delete',             NULL,  'Delete the organisation with all its users and records')
ON CONFLICT (key) DO NOTHING;

CREATE TABLE IF NOT EXISTS group_permissions (
    group_id   BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    permission TEXT   NOT NULL REFERENCES permissions(key) ON DELETE CASCADE,
    PRIMARY KEY (group_id, permission)
);

-- Existing groups: admin gets everything, every other group the member set.
INSERT INTO group_permissions (group_id, permission)
SELECT g.id, p.key FROM groups g CROSS JOIN permissions p
WHERE g.name = 'admin'
ON CONFLICT DO NOTHING;

INSERT INTO group_permissions (group_id, permission)
SELECT g.id, p.key FROM groups g CROSS JOIN permissions p
WHERE g.name <> 'admin'
  AND p.key IN ('record:create:own', 'record:read:own', 'record:update:own', 'record:delete:own',
                'user:read', 'user:update:own', 'group:read', 'organisation:read')
ON CONFLICT DO NOTHING;

-- Records know their organisation (NULL for legacy owners without one).
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_id_organisation_id_key;
ALTER TABLE users ADD CONSTRAINT users_id_organisation_id_key UNIQUE (id, organisation_id);

ALTER TABLE records ADD COLUMN IF NOT EXISTS organisation_id BIGINT REFERENCES organisations(id);

UPDATE records r SET organisation_id = u.organisation_id
FROM users u
WHERE u.id = r.user_id AND r.organisation_id IS NULL;

-- The record's organisation is always its owner's (skipped while NULL).
ALTER TABLE records DROP CONSTRAINT IF EXISTS records_user_organisation_fkey;
ALTER TABLE records
    ADD CONSTRAINT records_user_organisation_fkey
    FOREIGN KEY (user_id, organisation_id) REFERENCES users (id, organisation_id);

CREATE INDEX IF NOT EXISTS records_organisation_id_idx ON records (organisation_id);

COMMIT;
