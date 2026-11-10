-- Google OAuth identity for users.

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS google_id VARCHAR(64);

CREATE UNIQUE INDEX IF NOT EXISTS users_google_id_key
    ON users (google_id)
    WHERE google_id IS NOT NULL;
