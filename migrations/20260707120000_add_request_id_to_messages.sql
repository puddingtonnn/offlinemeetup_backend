-- +goose Up
-- +goose StatementBegin

-- request_id — клиентский idempotency key для надёжной отправки (retry/очередь).
-- Уникален в области (chat_id, sender_id): одна и та же тройка не создаёт дубль,
-- но тот же ключ, переиспользованный в ДРУГОМ чате, создаёт отдельное сообщение
-- (а не молча теряет его). Частичный индекс — старые/не-idempotent сообщения
-- имеют NULL и в него не попадают (в Postgres NULL в UNIQUE и так различны, но
-- partial ещё и не раздувает индекс).
ALTER TABLE messages ADD COLUMN request_id TEXT;

CREATE UNIQUE INDEX idx_messages_chat_sender_request_id
    ON messages (chat_id, sender_id, request_id)
    WHERE request_id IS NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_messages_chat_sender_request_id;
ALTER TABLE messages DROP COLUMN IF EXISTS request_id;
-- +goose StatementEnd
