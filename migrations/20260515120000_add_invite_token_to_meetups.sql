-- +goose Up
-- +goose StatementBegin
ALTER TABLE meetups ADD COLUMN invite_token UUID NOT NULL DEFAULT gen_random_uuid();
CREATE UNIQUE INDEX idx_meetups_invite_token ON meetups(invite_token);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX idx_meetups_invite_token;
ALTER TABLE meetups DROP COLUMN invite_token;
-- +goose StatementEnd
