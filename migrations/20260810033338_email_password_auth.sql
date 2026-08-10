-- +goose Up
-- +goose StatementBegin

-- 1. case-insensitive email uniqueness
UPDATE users SET email = lower(trim(email)) WHERE email IS NOT NULL;
CREATE UNIQUE INDEX uq_users_email_lower ON users (lower(email)) WHERE email IS NOT NULL;

-- 2. split nickname → username + display_name (ADR-4, ADR-5).
-- NOTE: constraint name below is Bun's default naming guess (profile_nickname_key),
-- NOT verified against a live DB in this environment — see task-1-report.md.
ALTER TABLE profile RENAME COLUMN nickname TO username;
ALTER TABLE profile ADD COLUMN display_name VARCHAR;
UPDATE profile SET display_name = username;
UPDATE profile SET username = 'user_' || user_id;
ALTER TABLE profile DROP CONSTRAINT profile_nickname_key;
CREATE UNIQUE INDEX uq_profile_username_lower ON profile (lower(username));

-- 3. passwords (ADR-6) — table only; nothing reads/writes it yet in this task
CREATE TABLE user_credentials (
    user_id       BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    password_hash TEXT NOT NULL,
    updated_at    TIMESTAMP NOT NULL DEFAULT current_timestamp
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS user_credentials;

DROP INDEX IF EXISTS uq_profile_username_lower;
ALTER TABLE profile ADD CONSTRAINT profile_nickname_key UNIQUE (username);
UPDATE profile SET username = display_name WHERE display_name IS NOT NULL;
ALTER TABLE profile DROP COLUMN display_name;
ALTER TABLE profile RENAME COLUMN username TO nickname;

DROP INDEX IF EXISTS uq_users_email_lower;

-- +goose StatementEnd
