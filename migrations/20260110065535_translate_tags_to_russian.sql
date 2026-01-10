-- +goose Up
-- +goose StatementBegin

UPDATE tags SET name = 'IT и технологии' WHERE name = 'IT & Tech';
UPDATE tags SET name = 'Бизнес'          WHERE name = 'Business';
UPDATE tags SET name = 'Нетворкинг'      WHERE name = 'Networking';
UPDATE tags SET name = 'Музыка'          WHERE name = 'Music';
UPDATE tags SET name = 'Спорт'           WHERE name = 'Sport';
UPDATE tags SET name = 'Образование'     WHERE name = 'Education';
UPDATE tags SET name = 'Игры'            WHERE name = 'Games';
UPDATE tags SET name = 'Искусство'       WHERE name = 'Art';

INSERT INTO tags (name) VALUES ('Еда'), ('Путешествия') ON CONFLICT (name) DO NOTHING;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

UPDATE tags SET name = 'IT & Tech'  WHERE name = 'IT и технологии';
UPDATE tags SET name = 'Business'   WHERE name = 'Бизнес';
UPDATE tags SET name = 'Networking' WHERE name = 'Нетворкинг';
UPDATE tags SET name = 'Music'      WHERE name = 'Музыка';
UPDATE tags SET name = 'Sport'      WHERE name = 'Спорт';
UPDATE tags SET name = 'Education'  WHERE name = 'Образование';
UPDATE tags SET name = 'Games'      WHERE name = 'Игры';
UPDATE tags SET name = 'Art'        WHERE name = 'Искусство';

-- DELETE FROM tags WHERE name IN ('Еда', 'Путешествия');

-- +goose StatementEnd