-- +goose Up
-- +goose StatementBegin
ALTER TABLE files ADD COLUMN uploaded_by BIGINT REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX idx_files_uploaded_by ON files(uploaded_by);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_files_uploaded_by;
ALTER TABLE files DROP COLUMN uploaded_by;
-- +goose StatementEnd
