-- Records belong to a user. Existing records are given to the user with the
-- lowest id. If records exist but there is no user yet, the migration fails
-- and changes nothing.
BEGIN;

ALTER TABLE records ADD COLUMN IF NOT EXISTS user_id BIGINT REFERENCES users(id);

UPDATE records
SET user_id = (SELECT MIN(id) FROM users)
WHERE user_id IS NULL;

ALTER TABLE records ALTER COLUMN user_id SET NOT NULL;

CREATE INDEX IF NOT EXISTS records_user_id_idx ON records (user_id);

COMMIT;
