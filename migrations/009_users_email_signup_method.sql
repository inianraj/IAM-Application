-- Users no longer have passwords: they are created without one and sign in with
-- their email ('email' method), or through a provider later ('google', 'github').
--
-- Existing 'password' and 'legacy' users become 'email' users, so they can sign
-- in by email. Their password hashes are dropped with the column.
BEGIN;

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_password_method_check;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_signup_method_check;

UPDATE users SET signup_method = 'email' WHERE signup_method IN ('password', 'legacy');

ALTER TABLE users DROP COLUMN IF EXISTS password_hash;

ALTER TABLE users
    ADD CONSTRAINT users_signup_method_check
    CHECK (signup_method IN ('email', 'google', 'github'));
ALTER TABLE users ALTER COLUMN signup_method SET DEFAULT 'email';

COMMIT;
