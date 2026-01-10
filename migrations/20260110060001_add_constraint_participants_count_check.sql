-- +goose Up
-- +goose StatementBegin
ALTER TABLE meetups ADD CONSTRAINT participants_count_check CHECK (participants_count >= 0);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE meetups DROP CONSTRAINT participants_count_check;
-- +goose StatementEnd
