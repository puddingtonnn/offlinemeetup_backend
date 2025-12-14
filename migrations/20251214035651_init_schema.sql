-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS postgis;

CREATE TABLE users (
                       id BIGSERIAL PRIMARY KEY,
                       email VARCHAR UNIQUE,
                       role VARCHAR DEFAULT 'user',
                       status VARCHAR(20) DEFAULT 'active',
                       created_at TIMESTAMP DEFAULT current_timestamp,
                       updated_at TIMESTAMP DEFAULT current_timestamp
);

CREATE TABLE social_accounts (
                                 id BIGSERIAL PRIMARY KEY,
                                 user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
                                 provider VARCHAR NOT NULL,
                                 social_id VARCHAR NOT NULL,
                                 created_at TIMESTAMP DEFAULT current_timestamp
);

CREATE TABLE profile (
                         id BIGSERIAL PRIMARY KEY,
                         user_id BIGINT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
                         nickname VARCHAR UNIQUE NOT NULL,
                         bio TEXT,
                         avatar_url VARCHAR NOT NULL,
                         is_organizer BOOLEAN DEFAULT FALSE,
                         updated_at TIMESTAMP DEFAULT current_timestamp
);

CREATE TABLE tags (
                      id BIGSERIAL PRIMARY KEY,
                      name VARCHAR UNIQUE NOT NULL
);

CREATE TABLE profile_tags (
                              profile_id BIGINT REFERENCES profile(id) ON DELETE CASCADE,
                              tag_id BIGINT REFERENCES tags(id) ON DELETE CASCADE,
                              PRIMARY KEY (profile_id, tag_id)
);

CREATE TABLE events (
                        id BIGSERIAL PRIMARY KEY,
                        title VARCHAR NOT NULL,
                        description TEXT,
                        is_public BOOLEAN NOT NULL DEFAULT TRUE,
                        created_at TIMESTAMP DEFAULT current_timestamp,
                        start_time TIMESTAMP NOT NULL,
                        end_time TIMESTAMP NOT NULL,
                        creator_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
                        location GEOGRAPHY(POINT, 4326)
);

CREATE INDEX idx_events_location ON events USING GIST (location);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS profile_tags;
DROP TABLE IF EXISTS tags;
DROP TABLE IF EXISTS profile;
DROP TABLE IF EXISTS social_accounts;
DROP TABLE IF EXISTS users;
-- +goose StatementEnd