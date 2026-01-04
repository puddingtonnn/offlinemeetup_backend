-- +goose Up
-- +goose StatementBegin

-- 1. Создаем связующую таблицу для митапов
CREATE TABLE meetup_tags (
    meetup_id BIGINT NOT NULL REFERENCES meetups(id) ON DELETE CASCADE,
    tag_id BIGINT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (meetup_id, tag_id)
);

-- 2. Наполняем справочник статическими данными.
-- Используем ON CONFLICT, чтобы миграция не упала, если теги уже есть.
INSERT INTO tags (name) VALUES 
('IT & Tech'),
('Business'),
('Networking'),
('Music'),
('Sport'),
('Education'),
('Games'),
('Art')
ON CONFLICT (name) DO NOTHING;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS meetup_tags;
-- Мы НЕ удаляем данные из tags в Down, так как они могут уже использоваться пользователями.
-- +goose StatementEnd