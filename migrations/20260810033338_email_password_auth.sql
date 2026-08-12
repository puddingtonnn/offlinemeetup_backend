-- +goose Up
-- +goose StatementBegin

-- 1. case-insensitive email uniqueness
UPDATE users SET email = lower(trim(email)) WHERE email IS NOT NULL;

-- Pre-flight guard: without this, two existing users whose emails differ only
-- by case would make the CREATE UNIQUE INDEX below fail with a bare
-- duplicate-key error, crash-looping the app at boot (goose.Up runs on every
-- startup). Fail with a clear, actionable message instead.
DO $$
DECLARE
    dup_count integer;
BEGIN
    SELECT count(*) INTO dup_count FROM (
        SELECT lower(email) FROM users WHERE email IS NOT NULL
        GROUP BY lower(email) HAVING count(*) > 1
    ) dups;
    IF dup_count > 0 THEN
        RAISE EXCEPTION 'Migration blocked: % email address(es) in users differ only by case and would collide under case-insensitive uniqueness. Resolve manually before this migration can proceed (see plan Verification section for the diagnostic query).', dup_count;
    END IF;
END $$;

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

-- Best-effort data restoration: display_name was never unique (users can
-- share the same display name), so this UPDATE runs before the UNIQUE
-- constraint is re-added — restoring as many original nicknames as possible
-- without a constraint blocking rows that happen to collide.
UPDATE profile SET username = display_name WHERE display_name IS NOT NULL;

-- This can fail with a duplicate-key error if the restoration above left two
-- or more rows sharing the same value (distinct users who had set the same
-- display_name). That is an accepted, documented data-loss risk of rolling
-- back this migration once real data has diverged — not a bug to work
-- around further; resolve any duplicates manually before rollback if needed.
ALTER TABLE profile ADD CONSTRAINT profile_nickname_key UNIQUE (username);

ALTER TABLE profile DROP COLUMN display_name;
ALTER TABLE profile RENAME COLUMN username TO nickname;

DROP INDEX IF EXISTS uq_users_email_lower;

-- +goose StatementEnd
