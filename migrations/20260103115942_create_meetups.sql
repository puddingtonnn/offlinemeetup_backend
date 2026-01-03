-- +goose Up
-- +goose StatementBegin
CREATE TABLE meetups (
                         id BIGSERIAL PRIMARY KEY,
                         title VARCHAR NOT NULL,
                         description TEXT,
                         cover_file_id UUID,
                         is_public BOOLEAN NOT NULL DEFAULT TRUE,
                         creator_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,

                         start_time TIMESTAMP NOT NULL,
                         end_time TIMESTAMP NOT NULL,

                         location GEOGRAPHY(POINT, 4326),
                         address_text TEXT,

                         created_at TIMESTAMP DEFAULT current_timestamp,
                         deleted_at TIMESTAMP
);

-- Индекс для быстрого гео-поиска (KNN - ближайшие соседи)
CREATE INDEX idx_meetups_location ON meetups USING GIST (location);
CREATE INDEX idx_meetups_start_time ON meetups (start_time);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_meetups_start_time;
DROP INDEX IF EXISTS idx_meetups_location;
DROP TABLE IF EXISTS meetups;
-- +goose StatementEnd
