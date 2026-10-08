-- Sign in with Google (#110).
--
-- users.google_sub stores Google's stable account identifier (the ID token
-- "sub" claim) once a user signs in with Google. It is the only Google data
-- SELAR keeps: the requested scopes are "openid email", and the profile name
-- and picture are neither requested nor stored. The email address continues
-- to live in users.email; a Google sign-in links to an existing account only
-- when Google reports the address as verified and it equals users.email.
--
-- Accounts created through Google get an unusable password hash (no password
-- matches it), so password sign-in stays impossible for them until a password
-- is set. Deleting the user deletes this column with the row.
ALTER TABLE users ADD COLUMN google_sub TEXT;

CREATE UNIQUE INDEX users_google_sub_key ON users (google_sub) WHERE google_sub IS NOT NULL;
