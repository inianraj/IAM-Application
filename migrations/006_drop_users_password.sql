-- Users no longer have a password: there is no login. This DELETES the stored
-- password hashes.
ALTER TABLE users DROP COLUMN IF EXISTS password_hash;
