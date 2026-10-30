-- Email users sign in with a password again: users.password_hash holds a bcrypt
-- hash. The column is nullable because 'google' / 'github' users never have
-- one, and because users that exist today have none yet.
--
-- EXISTING 'email' USERS CANNOT LOG IN after this migration until they get a
-- password (there is no reset endpoint yet; set one by hand, or recreate the
-- user / organisation).
BEGIN;

ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash TEXT;

COMMIT;
