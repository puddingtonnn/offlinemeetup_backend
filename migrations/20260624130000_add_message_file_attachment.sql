-- +goose Up
-- +goose StatementBegin
ALTER TABLE messages ADD COLUMN file_id UUID REFERENCES files(id) ON DELETE SET NULL;
CREATE INDEX idx_messages_file_id ON messages(file_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_messages_file_id;
ALTER TABLE messages DROP COLUMN file_id;
-- +goose StatementEnd
