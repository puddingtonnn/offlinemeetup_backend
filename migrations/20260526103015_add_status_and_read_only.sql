-- +goose Up
-- +goose StatementBegin
ALTER TABLE meetups ADD COLUMN status VARCHAR(20) NOT NULL DEFAULT 'active';
ALTER TABLE chats ADD COLUMN is_read_only BOOLEAN NOT NULL DEFAULT FALSE;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE meetups DROP COLUMN status;
ALTER TABLE chats DROP COLUMN is_read_only;
-- +goose StatementEnd
