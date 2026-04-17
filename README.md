# Meetuper Backend API

A robust REST API backend for the "Meetuper" mobile application, designed to help users organize, discover, and join offline meetups. Built with **Go**, this service leverages Clean Architecture principles and utilizes **PostGIS** for efficient spatial queries and location-based event discovery.

## Tech Stack

* **Language:** Go 1.24
* **Web Framework:** [chi](https://github.com/go-chi/chi) (lightweight, idiomatic router)
* **Database:** PostgreSQL 16 + **PostGIS** extension (for geographical data)
* **ORM / Query Builder:** [Bun](https://github.com/uptrace/bun)
* **Migrations:** [Goose](https://github.com/pressly/goose)
* **Infrastructure:** Docker, Docker Compose, GitHub Actions (CI/CD)
* **Documentation:** Swagger (swaggo)

## Architecture

The project strictly follows **Clean Architecture** patterns, separating concerns into distinct layers to ensure maintainability, testability, and scalability:

* `cmd/app/` — Entry point. Handles configuration initialization, DB connection, and server startup.
* `internal/transport/http/` — Presentation layer. Contains HTTP routing, handlers, middleware (auth, logging), and DTOs.
* `internal/service/` — Business logic layer. Handles core application rules, validations, and external API integrations (DaData, Google/Telegram Auth).
* `internal/repo/` — Data access layer. Manages database interactions using the Bun ORM.
* `internal/domain/` — Core business entities and domain models (Meetup, User, Profile, Location).
* `internal/config/` — Environment-based configuration management.

## Key Features

* **Spatial Search & Geolocation:**
  * Integrates with the **DaData API** for address suggestions and geocoding.
  * Uses PostGIS `ST_DWithin` and `ST_Distance` to query meetups within a specific radius of the user's coordinates.
* **Authentication & Authorization:**
  * Implements JWT-based session management.
  * Supports OAuth2 via Google ID Tokens and Telegram Login Widget integration.
* **Meetup Management:**
  * Full CRUD operations for events.
  * Concurrency-safe participant registration (Join/Leave mechanics).
* **Automated CI/CD:**
  * Configured GitHub Actions pipeline for building Docker images, pushing to Docker Hub, and deploying to a VPS via SSH.

## Getting Started

### Prerequisites
* Docker & Docker Compose
* Go 1.24 (for local execution without Docker)
* Make

### Configuration
Create a `.env` file in the root directory. You can use the following template:

```env
APP_PORT=8080
APP_ENV=local  # local, dev, prod

# Database Configuration
DB_DSN=postgres://user:password@localhost:5432/meetuper_db?sslmode=disable
POSTGRES_USER=user
POSTGRES_PASSWORD=password
POSTGRES_DB=meetuper_db

# Authentication secrets
JWT_SECRET_KEY=your_secret_key
GOOGLE_WEB_CLIENT_ID=your_google_client_id
TELEGRAM_BOT_TOKEN=your_telegram_bot_token

# External APIs
DADATA_TOKEN=your_dadata_api_key
```

### Running with Docker (Recommended)
The easiest way to spin up the application along with PostgreSQL (PostGIS) and Redis is via Docker Compose:

```bash
make up
```
The API will be available at `http://localhost:8080`.

### Running Locally (Development)
If you prefer to run the Go application directly on your host machine:

1. Start only the infrastructure containers (DB, Redis):
```bash
docker-compose -f docker-compose.dev.yml up -d meetuper_db meetuper_redis
```

2. Run database migrations:
```bash
make migration
```

3. Start the application:
```bash
make run
```

## API Documentation

This project uses Swagger for API documentation. Once the server is running, navigate to:
`http://localhost:8080/swagger/index.html`

To regenerate the documentation after making changes to the handlers, run:
```bash
make swag
```

## Makefile Commands
* `make build` — Compile the Go binary.
* `make deps` — Update dependencies (`go mod tidy`).
* `make migration NAME=xxx` — Generate a new SQL migration file.
* `make logs` — Tail logs for all Docker containers.
* `make clean` — Stop containers and remove database volumes.