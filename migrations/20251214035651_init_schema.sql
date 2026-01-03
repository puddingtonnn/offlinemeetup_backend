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

CREATE TABLE user_tags (
                              user_id BIGINT REFERENCES users(id) ON DELETE CASCADE,
                              tag_id BIGINT REFERENCES tags(id) ON DELETE CASCADE,
                              PRIMARY KEY (user_id, tag_id)
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS user_tags;
DROP TABLE IF EXISTS tags;
DROP TABLE IF EXISTS profile;
DROP TABLE IF EXISTS social_accounts;
DROP TABLE IF EXISTS users;
-- +goose StatementEnd