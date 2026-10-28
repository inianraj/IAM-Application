-- A user may exist without an organisation: users.group_id and
-- users.organisation_id become nullable, and either both are set or neither.
-- (The composite foreign key (group_id, organisation_id) -> groups is skipped
-- by PostgreSQL when a column is NULL, so the CHECK keeps the pair consistent;
-- when both are set the FK still guarantees the group belongs to the
-- organisation.)
--
-- Every organisation also gets a 'member' group, the basic role of users that
-- are created with an organisation_id. Existing organisations get it here;
-- new ones get it from OrganisationRepository.Create. Existing users keep
-- their groups.
BEGIN;

ALTER TABLE users ALTER COLUMN group_id DROP NOT NULL;
ALTER TABLE users ALTER COLUMN organisation_id DROP NOT NULL;

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_org_group_both_or_neither;
ALTER TABLE users
    ADD CONSTRAINT users_org_group_both_or_neither
    CHECK ((group_id IS NULL) = (organisation_id IS NULL));

INSERT INTO groups (organisation_id, name)
SELECT id, 'member' FROM organisations
ON CONFLICT DO NOTHING;

COMMIT;
