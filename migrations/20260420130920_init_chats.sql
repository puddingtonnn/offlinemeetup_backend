-- +goose Up
-- +goose StatementBegin

CREATE TABLE chats (
                       id SERIAL PRIMARY KEY,
                       type VARCHAR(20) NOT NULL,
                       meetup_id INTEGER REFERENCES meetups(id) ON DELETE CASCADE,
                       title VARCHAR(255),
                       created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
CREATE INDEX idx_chats_meetup_id ON chats(meetup_id);

CREATE TABLE chat_participants (
                                   chat_id INTEGER REFERENCES chats(id) ON DELETE CASCADE,
                                   user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
                                   joined_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
                                   last_read_message_id INTEGER DEFAULT 0,
                                   PRIMARY KEY (chat_id, user_id)
);
CREATE INDEX idx_chat_participants_user_id ON chat_participants(user_id);

CREATE TABLE messages (
                          id SERIAL PRIMARY KEY,
                          chat_id INTEGER NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
                          sender_id INTEGER NOT NULL REFERENCES users(id),
                          content TEXT NOT NULL,
                          message_type VARCHAR(20) DEFAULT 'text',
                          created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
CREATE INDEX idx_messages_chat_id_created_at ON messages(chat_id, created_at DESC);

-- +goose StatementEnd


-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS chat_participants;
DROP TABLE IF EXISTS chats;

-- +goose StatementEnd