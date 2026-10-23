-- Groups belong to an organisation, and group names are unique per
-- organisation. Every organisation gets an "admin" group, which new users
-- are signed up into.
--
-- This REMOVES all groups that have no organisation (those created before
-- this migration). Users in them are moved to the "admin" group of the
-- organisation with the lowest id, so no user is deleted. If users exist but
-- there is no organisation yet, the migration fails and changes nothing.
BEGIN;

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS organisation_id BIGINT
    REFERENCES organisations(id) ON DELETE CASCADE;

-- Names were globally unique; they are now unique per organisation (below).
ALTER TABLE groups DROP CONSTRAINT IF EXISTS groups_name_key;

INSERT INTO groups (organisation_id, name)
SELECT o.id, 'admin'
FROM organisations o
WHERE NOT EXISTS (
    SELECT 1 FROM groups g WHERE g.organisation_id = o.id AND g.name = 'admin'
);

UPDATE users
SET group_id = (
        SELECT g.id FROM groups g
        WHERE g.name = 'admin'
          AND g.organisation_id = (SELECT MIN(id) FROM organisations)
    ),
    updated_at = NOW()
WHERE group_id IN (SELECT id FROM groups WHERE organisation_id IS NULL);

DELETE FROM groups WHERE organisation_id IS NULL;

ALTER TABLE groups ALTER COLUMN organisation_id SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS groups_organisation_id_name_key
    ON groups (organisation_id, name);

COMMIT;
