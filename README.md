# offlinemeetup_backend

Бэкенд для мобильного приложения "Meetuper". Сервис предоставляет REST API для организации оффлайн-встреч, поиска событий по геолокации и управления профилями пользователей.

## Стек технологий

* **Язык:** Go 1.24
* **Веб-фреймворк:** [chi](https://github.com/go-chi/chi)
* **База данных:** PostgreSQL 16 + расширение **PostGIS** (гео-индексы)
* **ORM / Query Builder:** [Bun](https://github.com/uptrace/bun)
* **Миграции:** [Goose](https://github.com/pressly/goose)
* **Инфраструктура:** Docker, Docker Compose, GitHub Actions
* **Документация:** Swagger (swaggo)

## Архитектура

Проект построен по принципам чистой архитектуры (Clean Architecture) со стандартной слоистой структурой:

* `cmd/app/` — Точка входа (`main.go`). Инициализация конфига, БД и запуск сервера.
* `internal/transport/http/` — Слой HTTP. Роутинг, хендлеры и middleware (auth, logging).
* `internal/service/` — Бизнес-логика. Валидация данных, взаимодействие с внешними API (DaData, Google Auth).
* `internal/repo/` — Слой данных. Работа с БД через Bun.
* `internal/domain/` — Основные сущности приложения (Meetup, User, Profile).
* `internal/config/` — Загрузка конфигурации из env.

## Основные возможности

* **Авторизация:**
* Вход через Google (ID Token).
* Вход через Telegram (Login Widget).
* JWT-токены для сессий.


* **Митапы:**
* CRUD операции (создание, чтение, обновление, удаление).
* Гео-поиск: поиск ближайших митапов в радиусе (используется `ST_DWithin` и `ST_Distance`).
* Участие в митапах (Join/Leave).


* **Гео-сервисы:**
* Подсказки адресов через API DaData.
* Преобразование адреса в координаты.


* **Профиль:**
* Редактирование профиля, управление тегами (интересами).



## Установка и запуск

### Предварительные требования

* Docker & Docker Compose
* Go 1.24 (для локальной разработки без Docker)
* Make

### Конфигурация

Перед запуском необходимо создать файл `.env` в корне проекта. Пример переменных на основе `internal/config/config.go`:

```env
APP_PORT=8080
APP_ENV=local  # local, dev, prod

# База данных
DB_DSN=postgres://user:password@localhost:5432/meetuper_db?sslmode=disable
POSTGRES_USER=user
POSTGRES_PASSWORD=password
POSTGRES_DB=meetuper_db

# Авторизация
JWT_SECRET_KEY=your_secret_key
GOOGLE_WEB_CLIENT_ID=your_google_client_id
TELEGRAM_BOT_TOKEN=your_telegram_bot_token

# Внешние API
DADATA_TOKEN=your_dadata_api_key

```

### Запуск в Docker (рекомендуемый)

Разворачивает приложение, PostgreSQL (PostGIS) и Redis.

```bash
make up

```

Приложение будет доступно по адресу `http://localhost:8080`.

### Локальный запуск (для разработки)

Если требуется запустить Go-приложение вне контейнера (БД всё равно лучше поднять в Docker):

1. Поднять только базы:
```bash
docker-compose -f docker-compose.dev.yml up -d meetuper_db meetuper_redis

```


2. Накатить миграции (при запуске через `main.go` миграции накатываются автоматически, но можно и вручную):
```bash
make migration

```


3. Запустить приложение:
```bash
make run

```



## API Документация

В проекте используется Swagger. После запуска сервера документация доступна по адресу:
`http://localhost:8080/swagger/index.html`

Для перегенерации документации после изменения кода:

```bash
make swag

```

## Деплой

В репозитории настроен CI/CD через GitHub Actions (`.github/workflows/deploy.yml`).
Пайплайн:

1. Собирает Docker-образ.
2. Пушит в Docker Hub.
3. Подключается по SSH к VPS.
4. Обновляет контейнеры через `docker compose`.

**Важно:** Для продакшена используется `docker-compose.yml`, который настроен на работу с Traefik (метки `traefik.enable=true`, TLS через Let's Encrypt).

## Особенности и ограничения

1. **PostGIS:** Для работы БД требуется образ `postgis/postgis:16-3.4-alpine`. Стандартный образ Postgres не подойдет, так как используются гео-типы `GEOGRAPHY(POINT, 4326)`.
2. **DaData:** Для работы подсказок адресов (`/v1/geo/suggest`) обязателен валидный токен DaData. Без него эндпоинт будет возвращать ошибку.
3. **Soft Delete:** Митапы удаляются "мягко" (проставляется `deleted_at`), но в текущей реализации репозитория `Delete` выполняет физическое удаление (`r.db.NewDelete()`). Стоит проверить, соответствует ли это бизнес-требованиям (в модели поле `DeletedAt` есть).

## Полезные команды (Makefile)

* `make build` — Сборка бинарника.
* `make deps` — Обновление зависимостей (`go mod tidy`).
* `make migration NAME=xxx` — Создание новой SQL миграции.
* `make logs` — Просмотр логов контейнеров.
* `make clean` — Полная очистка (остановка контейнеров и удаление вольюмов БД).