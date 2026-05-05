-- +goose Up
-- +goose StatementBegin
CREATE TABLE files (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    file_name VARCHAR NOT NULL,
    key VARCHAR NOT NULL UNIQUE,
    bucket VARCHAR NOT NULL,
    size BIGINT NOT NULL,
    mime_type VARCHAR NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT current_timestamp
);

ALTER TABLE profile ADD COLUMN avatar_file_id UUID REFERENCES files(id) ON DELETE SET NULL;
ALTER TABLE profile DROP COLUMN avatar_url;

ALTER TABLE meetups ADD CONSTRAINT fk_meetups_cover_file FOREIGN KEY (cover_file_id) REFERENCES files(id) ON DELETE SET NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE meetups DROP CONSTRAINT IF EXISTS fk_meetups_cover_file;

ALTER TABLE profile ADD COLUMN avatar_url VARCHAR;
ALTER TABLE profile DROP COLUMN avatar_file_id;

DROP TABLE IF EXISTS files;
-- +goose StatementEnd
