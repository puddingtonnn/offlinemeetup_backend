-- +goose Up
-- +goose StatementBegin
ALTER TABLE messages ADD COLUMN edited_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE messages ADD COLUMN deleted_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE messages ADD COLUMN reply_to_message_id INTEGER REFERENCES messages(id) ON DELETE SET NULL;
CREATE INDEX idx_messages_reply_to ON messages(reply_to_message_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_messages_reply_to;
ALTER TABLE messages DROP COLUMN reply_to_message_id;
ALTER TABLE messages DROP COLUMN deleted_at;
ALTER TABLE messages DROP COLUMN edited_at;
-- +goose StatementEnd
