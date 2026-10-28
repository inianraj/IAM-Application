-- Users get an optional password hash and the method they signed up with.
-- password_hash is NULL for users who signed up through a provider (google,
-- github) and so have no password.
--
-- Existing users were created while there was no password, so they are marked
-- 'legacy': no hash, and they cannot log in until a set-password flow exists.
BEGIN;

ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS signup_method TEXT NOT NULL DEFAULT 'legacy';
ALTER TABLE users ALTER COLUMN signup_method DROP DEFAULT;

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_signup_method_check;
ALTER TABLE users
    ADD CONSTRAINT users_signup_method_check
    CHECK (signup_method IN ('password', 'google', 'github', 'legacy'));

-- A password user must have a hash; every other method must not.
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_password_method_check;
ALTER TABLE users
    ADD CONSTRAINT users_password_method_check
    CHECK ((signup_method = 'password') = (password_hash IS NOT NULL));

COMMIT;
