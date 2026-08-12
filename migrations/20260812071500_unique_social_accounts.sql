-- +goose Up
-- +goose StatementBegin

-- (provider, social_id) is the lookup key of every social login
-- (UserRepo.GetBySocialID) and was never enforced as unique, so two
-- concurrent first-logins could leave duplicate rows for one Google/Telegram
-- identity — after which GetBySocialID's single-row Scan picks an arbitrary
-- one of the two accounts. UserRepo.LinkSocialAccount also needs this index
-- as its ON CONFLICT target.
--
-- Pre-flight guard, same shape as the email one in
-- 20260810033338_email_password_auth: goose.Up runs on every startup, so a
-- bare duplicate-key failure here would crash-loop the app with an
-- unactionable error.
DO $$
DECLARE
    dup_count integer;
BEGIN
    SELECT count(*) INTO dup_count FROM (
        SELECT provider, social_id FROM social_accounts
        GROUP BY provider, social_id HAVING count(*) > 1
    ) dups;
    IF dup_count > 0 THEN
        RAISE EXCEPTION 'Migration blocked: % duplicate (provider, social_id) pair(s) in social_accounts. Resolve manually (keep one row per pair) before this migration can proceed.', dup_count;
    END IF;
END $$;

CREATE UNIQUE INDEX uq_social_accounts_provider_social_id
    ON social_accounts (provider, social_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS uq_social_accounts_provider_social_id;

-- +goose StatementEnd
