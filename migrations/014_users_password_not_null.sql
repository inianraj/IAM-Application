-- Every user has a password: users.password_hash becomes NOT NULL.
--
-- DESTRUCTIVE: users without a hash (every user created before migration 013)
-- are deleted, together with their records, role assignments, and any
-- organisation left without users (its groups and roles go with it). Register a
-- new organisation afterwards. Future google/github users must be stored with
-- an unusable random hash.
BEGIN;

DELETE FROM records WHERE user_id IN (SELECT id FROM users WHERE password_hash IS NULL);
DELETE FROM users WHERE password_hash IS NULL;
DELETE FROM organisations o WHERE NOT EXISTS (SELECT 1 FROM users u WHERE u.organisation_id = o.id);

ALTER TABLE users ALTER COLUMN password_hash SET NOT NULL;

COMMIT;
