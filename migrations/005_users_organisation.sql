-- Users store their organisation. A composite foreign key makes the database
-- guarantee that a user's group belongs to the user's organisation, so the
-- two can never disagree (even if a group is edited or two requests race).
BEGIN;

ALTER TABLE users ADD COLUMN IF NOT EXISTS organisation_id BIGINT;

UPDATE users u
SET organisation_id = g.organisation_id
FROM groups g
WHERE g.id = u.group_id AND u.organisation_id IS NULL;

ALTER TABLE users ALTER COLUMN organisation_id SET NOT NULL;

-- Target of the composite foreign key below.
CREATE UNIQUE INDEX IF NOT EXISTS groups_id_organisation_id_key
    ON groups (id, organisation_id);

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_group_org_fkey;
ALTER TABLE users
    ADD CONSTRAINT users_group_org_fkey
    FOREIGN KEY (group_id, organisation_id) REFERENCES groups (id, organisation_id);

-- Redundant now that the composite key covers group_id.
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_group_id_fkey;

COMMIT;
