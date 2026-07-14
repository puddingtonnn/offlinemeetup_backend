-- +goose Up
-- +goose StatementBegin

-- Приводим id/FK чат-таблиц к int8 (BIGINT). users.id и meetups.id — BIGSERIAL
-- (int8), а модели Go везде int64; chats/messages/chat_participants же были
-- созданы как SERIAL/INTEGER (int4). Рассинхрон типов + потолок переполнения
-- при id > 2^31-1. Расширение int4->int8 совместимо со старыми значениями.

ALTER TABLE chats ALTER COLUMN id TYPE BIGINT;
ALTER TABLE chats ALTER COLUMN meetup_id TYPE BIGINT;
ALTER SEQUENCE chats_id_seq AS BIGINT;

ALTER TABLE chat_participants ALTER COLUMN chat_id TYPE BIGINT;
ALTER TABLE chat_participants ALTER COLUMN user_id TYPE BIGINT;
ALTER TABLE chat_participants ALTER COLUMN last_read_message_id TYPE BIGINT;

ALTER TABLE messages ALTER COLUMN id TYPE BIGINT;
ALTER TABLE messages ALTER COLUMN chat_id TYPE BIGINT;
ALTER TABLE messages ALTER COLUMN sender_id TYPE BIGINT;
ALTER SEQUENCE messages_id_seq AS BIGINT;

-- Keyset-пагинация истории идёт по (chat_id, id DESC), а существующий индекс —
-- по (chat_id, created_at). Добавляем индекс под реальный порядок запроса.
CREATE INDEX IF NOT EXISTS idx_messages_chat_id_id ON messages (chat_id, id DESC);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_messages_chat_id_id;
-- Сужение BIGINT -> INTEGER потенциально теряет данные, поэтому вниз откатываем
-- только индекс; расширенные типы остаются (они обратно совместимы).

-- +goose StatementEnd
