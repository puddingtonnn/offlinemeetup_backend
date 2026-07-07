-- +goose NO TRANSACTION

-- Индекс под "мои созданные митапы" (List: WHERE creator_id = ?) и под authz-путь
-- GetForAuth. Без него запрос идёт seq-scan по meetups. CONCURRENTLY + NO
-- TRANSACTION: построение индекса не блокирует запись в таблицу (важно, т.к.
-- миграции применяются на старте процесса), но требует запуска вне транзакции.

-- +goose Up
-- +goose StatementBegin
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_meetups_creator_id ON meetups (creator_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX CONCURRENTLY IF EXISTS idx_meetups_creator_id;
-- +goose StatementEnd
