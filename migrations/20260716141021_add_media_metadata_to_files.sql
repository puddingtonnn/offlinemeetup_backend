-- +goose Up
-- +goose StatementBegin
ALTER TABLE files
    ADD COLUMN duration_ms BIGINT,
    ADD COLUMN width INT,
    ADD COLUMN height INT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE files
    DROP COLUMN duration_ms,
    DROP COLUMN width,
    DROP COLUMN height;
-- +goose StatementEnd
