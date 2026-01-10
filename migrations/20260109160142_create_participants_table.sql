-- +goose Up
-- +goose StatementBegin
CREATE TABLE participants (
    meetup_id BIGINT NOT NULL REFERENCES meetups(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role VARCHAR(20) NOT NULL DEFAULT 'member',
    status VARCHAR(20) NOT NULL DEFAULT 'approved',
    last_read_message_id BIGINT,
    is_muted BOOLEAN NOT NULL DEFAULT FALSE,
    joined_at TIMESTAMP DEFAULT current_timestamp,
    PRIMARY KEY (meetup_id, user_id)
);

CREATE INDEX idx_participants_user_id ON participants(user_id);
ALTER TABLE meetups ADD COLUMN participants_count INT NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE meetups DROP COLUMN IF EXISTS participants_count;
DROP TABLE IF EXISTS participants;
-- +goose StatementEnd
