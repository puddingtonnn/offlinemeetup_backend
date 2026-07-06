-- +goose Up
-- +goose StatementBegin

-- meetups.participants_count теперь ведёт БД, а не Go-код. Триггер на
-- participants ловит и обычные Join/Leave, и каскадные удаления (ON DELETE
-- CASCADE при удалении юзера), где ручной Go-счётчик дрейфовал.

-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION meetup_participants_count_sync() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        UPDATE meetups SET participants_count = participants_count + 1 WHERE id = NEW.meetup_id;
    ELSIF TG_OP = 'DELETE' THEN
        UPDATE meetups SET participants_count = participants_count - 1 WHERE id = OLD.meetup_id;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER trg_participants_count
    AFTER INSERT OR DELETE ON participants
    FOR EACH ROW EXECUTE FUNCTION meetup_participants_count_sync();
-- +goose StatementEnd

-- +goose StatementBegin
-- Разовая сверка: чиним любой уже накопившийся дрейф.
UPDATE meetups m SET participants_count = (
    SELECT COUNT(*) FROM participants p WHERE p.meetup_id = m.id
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS trg_participants_count ON participants;
DROP FUNCTION IF EXISTS meetup_participants_count_sync();
-- +goose StatementEnd
