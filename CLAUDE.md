# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

REST + WebSocket backend for "Meetuper", a mobile app for organizing offline meetups. Go 1.26, Chi router, PostgreSQL (PostGIS) via Bun ORM, Redis for caching/pub-sub, S3-compatible storage (MinIO) for files.

## Commands

All common tasks go through the `Makefile`:

- `make run` — run locally (`go run cmd/app/main.go`); needs a reachable DB/Redis (see gotchas below)
- `make build` — build binary to `bin/meetuper`
- `make test` — `go test -v ./...`
- `make lint` — `golangci-lint run`
- `make up` / `make down` / `make logs` / `make restart` — Docker dev environment (`docker-compose.dev.yml`)
- `make clean` — stop containers and remove volumes (drops DB data)
- `make migration NAME=create_x_table` — scaffold a new goose SQL migration in `migrations/`
- `make swag` — regenerate Swagger docs in `docs/` from annotations

Run a single test: `go test ./internal/service/ -run TestMeetupService_CreateMeetup -v`

Swagger UI is served at `/swagger/*`; health check at `/health`.

## Architecture

Layered architecture; dependencies are wired manually in `internal/app/app.go` (`New`), which constructs repos → services → handlers → router. Request flow:

```
transport/http/handler  →  service  →  repo  →  Bun/Postgres
   (DTO ↔ domain)        (business logic)  (SQL)
```

- **`internal/domain/`** — Bun models (structs with `bun:` tags) and relations. The core data types passed between layers.
- **`internal/repo/`** — data access via Bun. One file per aggregate (user, profile, meetup, chat, tag, file).
- **`internal/service/`** — business logic. **Each service file declares the repository interface it depends on** (e.g. `MeetupRepository` is defined in `service/meetup.go`, not in `repo/`). This is the seam used for mocking.
- **`internal/transport/http/`** — `router.go` (all routes + middleware groups), `handler/` (HTTP handlers, Swagger annotations), `dto/` (request/response shapes), `middleware/`, `response/`.
- **`internal/transport/websocket/`** — real-time chat. See WebSocket section.
- **`internal/config/config.go`** — env-var config loaded via `godotenv`; `Config` struct is passed everywhere it's needed.

### Errors

Services return sentinel errors from `internal/service/errors.go` (`ErrNotFound`, `ErrForbidden`, `ErrMeetupFinished`, `ErrChatReadOnly`, etc.). Handlers map these to HTTP status codes. Add new cross-cutting error categories there rather than ad-hoc error strings.

When a check lives **inside the repo** (e.g. an atomic transactional guard like read-only / membership in `repo.SaveMessage`), the repo returns its **own** package-level sentinel (`repo.ErrChatReadOnly`, `repo.ErrNotChatMember`) — it must not import `service`. The service then **translates** that infra error into a domain sentinel at the layer boundary (`mapChatRepoError` in `service/chat.go`) via `errors.Is`, so the dependency direction stays service→repo and handlers still see only `service.Err*`. Don't string-match error text.

### Auth & middleware

Two JWT middlewares in `internal/transport/http/middleware/auth.go`, applied per route group in `router.go`:
- `AuthMiddleware` — **requires** a valid `Bearer` token; 401 otherwise. User ID stored in context.
- `UserIdentityMiddleware` — **optional** auth; populates user ID if a token is present, otherwise passes through anonymously (used for public reads like meetup list/detail so they can reflect the caller's membership).

Read the user ID with `middleware.GetUserIDFromContext(ctx)`. The JWT claim key is `userID` (int64).

**Access + refresh tokens.** Login (`google`/`telegram`/`dev`) returns `{access_token, refresh_token, expires_in}` (`dto.AuthTokensResponse`). The **access token** is a short-lived HS256 JWT (`JWT_ACCESS_TTL`, default 15m; claims `userID`/`iat`/`exp`) — that's the `Bearer` token the middlewares validate. The **refresh token** is an opaque 32-byte random string; only its SHA-256 hash is stored in `refresh_tokens` (`JWT_REFRESH_TTL`, default 30d). `POST /v1/auth/refresh` **rotates** it (revoke the presented token, issue a new pair) with **reuse-detection**: presenting an already-revoked token revokes every token of that user (`RevokeAllForUser`) and returns 401. `POST /v1/auth/logout` revokes a refresh token (idempotent). All token issuance funnels through `AuthService.issueTokenPair`; login methods return `*service.TokenPair`, not a bare string. The Telegram deep-link carries both tokens (`access_token`/`refresh_token`, both `url.QueryEscape`d).

`/auth/dev/login` is registered **only** when `APP_ENV` is `local` or `dev`.

### Security conventions

These are load-bearing — keep them when touching the relevant code:

- **Required secrets fail-fast.** `config.Load` returns an error (refuses to start) when `JWT_SECRET_KEY` or `TELEGRAM_BOT_TOKEN` is empty, in **every** environment (an empty HMAC key would let anyone forge tokens / Telegram signatures). Add new mandatory secrets the same way (mirror the `DB_DSN` check), not as a `WARNING`. Tests that call `config.Load()` must set both via `t.Setenv` (`setRequiredSecrets` helper).
- **`invite_token` is creator-only.** It's a join capability, so it must never reach non-creators/anonymous callers. The cached meetup snapshot stays caller-invariant (stores the full token); it's hidden on a **copy** at read time — `MeetupCache.Meetup` blanks it when `CreatorID != userID` (same overlay pattern as `IsMember`), and every other DTO egress (`ListMeetups`, chat-embedded meetup) calls `gateInviteToken(resp, callerID)` (`service/mappers.go`). `UpdateMeetup` **rotates** the token (`uuid.New()`) on a `public→private` transition. Never emit it ungated.
- **Per-message/-cover/-avatar authorization is enforced in the repo transaction**, and the service translates the repo sentinel to a domain one at the boundary (`mapChatRepoError`, `mapMeetupRepoError`, inline in `profile.go`). `WS messagesRead` goes through `ChatService.MarkAsRead` → `repo.MarkAsRead`, which **checks membership via `EXISTS`** (like `SaveMessage`) and returns `repo.ErrNotChatMember` → `ErrForbidden`; the WS handler re-marshals a **server-built** `WSMessagesReadPayload` (never reflect the client's `event.Payload`).
- **A reply must target a message in the *same* chat.** `repo.SaveMessage` verifies `reply_to_message_id`'s `chat_id == msg.ChatID` in-tx (else `ErrMessageNotFound`). `messageByID` loads the `ReplyTo` preview **without** a chat/membership filter, so without this guard a member of chat A could set `reply_to_message_id` to a message in a private chat B and exfiltrate its body via the reply preview.
- **Chat membership mirrors meetup participation.** `MeetupRepo.Join` adds the user to `chat_participants`; `MeetupRepo.Leave` **must** remove them (`ChatRepo.RemoveParticipant`, same tx) — otherwise someone who left a meetup keeps passing every `EXISTS(chat_participants)` check and retains full chat read/write. `MeetupRepo.Update` uses `.ExcludeColumn("participants_count","status","creator_id","created_at")` so a read-modify-write of the body can't clobber the counter or immutable columns.
- **`meetups.participants_count` is maintained by a DB trigger** (`trg_participants_count` on `participants`, AFTER INSERT/DELETE), **not** by Go. Do not add manual `+1`/`-1` in `Join`/`Leave`/`Create` — that would double-count. The trigger also catches cascade deletes (e.g. `ON DELETE CASCADE` when a user is removed), which the old manual counter missed.
- **File ownership.** `files.uploaded_by` records the uploader (set by `FileService.Upload`, which takes `userID`). Referencing a file you don't own (`cover_file_id`/`avatar_file_id`/message `file_id`) is rejected: the repo write paths call `fileOwnedBy(ctx, idb, fileID, userID)` and return `repo.ErrFileNotOwned` (→ `ErrForbidden`). This supersedes the old "no file-ownership check" note.
- **Uploads are validated by real bytes, not the client `Content-Type`/filename.** `FileService.Upload` sniffs the first 512 bytes via `internal/service/media` (`media.Detect`), which gates to a **media whitelist** (photo/video/audio) and derives **both** the stored `Content-Type` and the object-key extension from the detected type. The upload stream must be seekable (`io.ReadSeeker`): the head is read, then the stream is rewound so the S3 SDK signs and streams the body without buffering the whole file in RAM. Max size is `config.MaxUploadSize` (`MAX_UPLOAD_SIZE`, default 100 MB), enforced in the handler (`http.MaxBytesReader`) and the service. **Covers/avatars are image-only**: the shared `/files/upload` accepts any media, so the cover/avatar reference paths call `imageFileOwnedBy` in-tx (ownership **and** `mime_type LIKE 'image/%'`), returning `repo.ErrFileNotImage` (→ `ErrInvalidInput`). Chat attachments accept any media type.
- **Global middleware** (`router.go`, in order): `SecurityHeaders` (nosniff / `X-Frame-Options: DENY` / `Referrer-Policy` / `Cache-Control: no-store`) then `BodyLimit`. `RateLimiter(rdb, log, scope, limit, window, trustProxy)` (Redis fixed-window, shared across instances, fail-open) wraps the `/auth/*` group and the `/files/upload` route. Swagger (`/swagger/*`) is gated to `local`/`dev` like dev-login. **The limiter keys on the client IP; `X-Real-IP`/`X-Forwarded-For` are trusted ONLY when `TRUST_PROXY_HEADERS=true` (`config.TrustProxyHeaders`), else it falls back to `RemoteAddr`** — those headers are client-spoofable, so honoring them unconditionally lets anyone mint a fresh bucket per request. Enable it only behind a proxy that overwrites them.

### Caching

Redis caching lives behind `internal/cache`, not in raw `*redis.Client` use scattered through services:

- **`cache.Cache`** — minimal `Get`/`Set`/`Del` seam over the store. `Get` returns `(value, found, err)` so a miss (`found=false`) is distinct from a backend error.
- **`cache.RedisCache`** — the Redis implementation; logs read/write failures and is **best-effort** (a cache failure never fails the request).
- **`cache.Load[T](ctx, c, metrics, name, key, ttl, load)`** — generic cache-aside helper (get → JSON-decode → on miss/decode-error/backend-error call the loader, encode, set). It also: collapses concurrent misses for the same key via a shared `singleflight.Group` (anti-stampede); applies ±10% TTL **jitter** before `Set` (anti-avalanche, `jitter.go`); records hit/miss/error/latency via the `name` label. Use this instead of hand-rolling the dance. Two guards: it **does not cache a `"null"` value** (a pointer loader returning `(nil,nil)` would otherwise poison the key for the whole TTL — pointer loaders should still return `ErrNotFound`, not `(nil,nil)`), and `Set` runs on `context.WithoutCancel(ctx)` so a cancelled/expiring request doesn't suppress the cache fill. A backend `Get` error counts `Error` only (not also `Miss`).
- **`cache.NewTimeoutCache(inner, timeout)`** — decorator that bounds every Redis op with `context.WithTimeout` (default `CACHE_TIMEOUT=200ms`). A timeout surfaces as a `Get` error → `Load` treats it as a miss and degrades to the loader, so a hung Redis never blocks a request. Wire it around `RedisCache` in `app.go`.
- **`cache.Metrics`** — consumer-side seam (`Hit`/`Miss`/`Error`/`ObserveLatency`); `cache.NopMetrics` is the no-op used in tests. The Prometheus impl lives in a **separate package** `internal/cache/cachemetrics` (so `package cache` never imports Prometheus); `app.go` builds a non-global `prometheus.NewRegistry()` and exposes `GET /metrics`.
- **Key formats live in exactly one place** (`keys.go`: `UserChatsKey`, `TagsKey`, `ProfileKey`, `MeetupKey`). Never build a cache key with an inline `fmt.Sprintf` in a service.
- **Domain cache helpers** (`ChatCache`, `TagCache`, `ProfileCache`, `MeetupCache`) own a key + TTL (from config) + metrics and expose intent-named methods. A service depends on a **narrow interface declared at the consumer** (`chatCacheInvalidator`, `tagCache`, `profileCache`, `meetupCache`), not on `*redis.Client` or the cache type — so caching/invalidation is an explicit dependency, not a leaked key string. **Cached endpoints:** chats list, `GET /v1/tags`, `GET /v1/profile/{id}`, `GET /v1/meetups/{id}`.
- **Per-user fields + a shared cache entry:** `MeetupCache` caches one **caller-invariant** snapshot per meetup (loader calls `repo.GetByID(ctx, id, 0)` so `is_member` is not computed) plus the member `user_id` set taken from **domain** participants (independent of whether they have a profile — the DTO mapper drops profile-less participants, so never derive membership from the DTO). The per-caller `IsMember` is overlaid onto a **copy** of the response at read time (never mutate the shared cached object — singleflight may hand it to several callers). The `GetMeetup` authz check runs on that overlaid copy. Invalidate `meetup:{id}` on every body/participant mutation (update/delete/join/leave/join-by-token).

When adding a new cached read: add a key builder in `keys.go`, add a base-TTL field to `Config`, add a typed domain-cache helper that forwards to `cache.Load`, declare a narrow consumer interface at the service, and invalidate via that helper on the mutating paths.

### WebSocket / real-time chat

Hub-and-client pattern (`internal/transport/websocket/`). A single `Hub` runs in a goroutine (started in `App.Run`), keyed by `userID`, with channels for register/unregister/broadcast. Each connection has a `Client` with `readPump`/`writePump` goroutines. HTTP handlers (e.g. `chatHandler.SendMessage`) persist the message and call `hub.BroadcastToUsers(...)` to push to online recipients. `WS-EXPLANATION.md` has a line-by-line walkthrough; `websocket.md` documents the event protocol.

**Message attachments.** A message can reference one uploaded file via `messages.file_id` (`uuid`, FK `files(id)`, `ON DELETE SET NULL`) — the same upload-then-reference pattern as `cover_file_id`/`avatar_file_id` (`POST /v1/files/upload` → `id` → put it in the send payload). `SendMessage` takes an optional `fileID *string` (UUID), parses it, sets `message_type="file"`, and **allows empty `content`** when an attachment is present (caption optional). The DTO mapper adds an `Attachment {url, file_name, mime_type, size}` built with `publicURL(s3PublicURL, m.File)`; the precise image-vs-file rendering is left to the client via `mime_type`. A soft-deleted message hides its attachment too. Like the cover/avatar flow, the attachment **must belong to the sender** — `repo.SaveMessage` enforces `fileOwnedBy(...)` in-tx (see Security conventions).

**Message edit / delete / reply.** `messages` has `edited_at`, `deleted_at` (soft-delete tombstone) and `reply_to_message_id` (self-FK, `ON DELETE SET NULL`). Edit/delete are author-only and enforced **atomically in the repo**: `lockMessage` does `SELECT ... FOR UPDATE`, mapping a missing/soft-deleted row to `repo.ErrMessageNotFound` and a non-author to `repo.ErrNotMessageAuthor` (translated to `ErrNotFound`/`ErrForbidden` by `mapChatRepoError`). `EditMessage`/`DeleteMessage` also take the `chatID` from the URL and reject a message whose `chat_id` doesn't match (→ `ErrMessageNotFound`), so the `{id}` path segment is bound to the message, not ignored. The DTO mapper (`mapMessageToResponse`) **blanks the body of a soft-deleted message** (`IsDeleted=true`, `Content=""`) — never leak a deleted body, including in a reply preview. `ReplyTo` is loaded one level deep (sender + nickname) via `messageByID`/`GetMessages`. Edit/delete are REST (`PATCH`/`DELETE /v1/chats/{id}/messages/{messageId}`), not WS client events; the server broadcasts `messageEdited` (full DTO) / `messageDeleted` (`{chat_id, message_id}`) to participants. `SendMessage` takes an optional `replyToMessageID *int64`.

**Presence (online/offline + last seen).** Source of truth is Redis (connections are spread across devices and instances). `cache.RedisPresenceStore` keeps a Redis **set** of connection IDs per user (`presence:conns:{id}`); a user is online while `SCARD > 0`. Offline↔online **transitions** are computed atomically by Lua scripts (`SADD`+`SCARD==1` / `SREM`+`SCARD==0`) so two instances racing the first/last connection don't both broadcast. The set carries a TTL (`PRESENCE_TTL`, default 2m) refreshed by a heartbeat on the `writePump` ping tick — a crashed instance's connections decay to offline instead of sticking. `service.PresenceService` owns the policy (who to notify = co-chat members via `ChatService.GetCoChatUserIDs`; membership checks for `StatusForChat`) and **does not import the websocket package**: it returns `(becameOnline, recipients)` and the transport layer builds the WS event and broadcasts — keeping the dependency direction transport→service. Lifecycle: `OnConnect` in `ServeWs` (+ a `presenceSnapshot` pushed to the new client), `OnDisconnect` in `readPump`'s defer keyed by a per-connection `connID` (idempotent `SREM`, so a backpressure-drop + readPump exit can't double-fire; uses a fresh `context.Background()` because the connection ctx is already cancelled). Events: `userOnline`, `userOffline`, `presenceSnapshot`; REST `GET /v1/chats/{id}/presence`. Presence is best-effort — a Redis failure never breaks the chat connection.

**Connection teardown / channel ownership.** `client.send` has **multiple senders** (hub broadcasts via `trySend`, `sendError`, the presence snapshot in `announcePresence`), so it is **never closed** — closing a channel that other goroutines still send to panics (`send on closed channel`). Teardown goes through `client.stop()`, which cancels the connection ctx: `writePump` exits on `ctx.Done` and closes the socket, which in turn unblocks `readPump`. The hub's `unregister` case and shutdown (`ctx.Done`) call `client.stop()`, **not** `close(client.send)`. Do not reintroduce `close(client.send)` — that was a real panic vector.

**Multi-instance scaling (Redis Pub/Sub).** Broadcasts fan out across backend instances through a `MessageBus` seam (`broker.go`): `BroadcastToUsers`/`BroadcastToRooms` only **publish** a `busEnvelope` to the Redis channel `ws:broadcast` — they no longer touch the local `broadcast` channel directly. Each instance runs `Hub.StartConsumer(ctx)` (alongside `Hub.Run`, both in `App.Run`) which subscribes and feeds decoded broadcasts into `Run` for local delivery. **Local delivery happens ONLY in the consumer**, so a message is delivered exactly once even though the publishing instance also receives its own publish. Consequences: (1) room-exclusion uses `SenderUserID int64`, not a `*Client` pointer (the sender may be on another instance); (2) `busEnvelope.Payload` is `json.RawMessage` — WS payloads must be valid JSON (they always are: marshaled `WSEvent`s); (3) Redis is now a hard dependency for real-time delivery (best-effort publish, but no consumer = no delivery). `NewRedisBus(rdb)` in prod; `NewLocalBus()` (in-process) for tests. Delivery is at-most-once — the source of truth is the DB and clients backfill history via REST on reconnect.

## Migrations

SQL migrations in `migrations/` are managed by goose and **embedded** (`migrations/embed.go`). `cmd/app/main.go` runs `goose.Up` automatically on every startup before the app boots — there is no separate migrate step. Create new ones with `make migration NAME=...`.

## Testing

- Service-layer tests use **gomock** (`go.uber.org/mock`) with **miniredis** for an in-memory Redis (see `internal/service/meetup_test.go` for the `setupXTest` pattern).
- Mocks are generated by `mockgen` (`//go:generate`-style header comments in each mock file show the exact command). Repository-interface mocks live in `internal/repo/mocks/`; service-interface mocks (consumed by handlers/WS) live in `internal/service/mocks/`. Regenerate after changing an interface, e.g.:
  `mockgen -source=internal/service/meetup.go -destination=internal/repo/mocks/meetup_mock.go -package=mocks`

## Gotchas

- **Redis address** comes from `REDIS_ADDR` (also `REDIS_PASSWORD`, `REDIS_DB`), defaulting to `meetuper_redis:6379` in `internal/config/config.go`. `make run` outside Docker needs `REDIS_ADDR=localhost:6379` (or a reachable host) — otherwise it falls back to the Docker hostname; `make up` works out of the box.
- Config is **env-driven** (`.env` for local). `DB_DSN` is required; missing Google/Telegram/JWT/DaData vars only print warnings. Default port 9090, default env `local`.
- The DB image is **PostGIS** (`postgis/postgis`); location data uses `twpayne/go-geom`. Keep geo columns/queries PostGIS-compatible.
- File uploads (`POST /v1/files/upload`) return a file `id`; create/update requests reference `cover_file_id` / `avatar_file_id` (not raw URLs). Responses build full public URLs as `S3_PUBLIC_URL + key`. See `REFAC-REPORT.md`.
- Stray top-level files (`taks.go`, `main`, `test_jwt*.go`) are scratch/build artifacts, not part of the app entrypoint — the real entrypoint is `cmd/app/main.go`.
- `MAX_UPLOAD_SIZE` (bytes, default `104857600` = 100 MB) caps media uploads. Uploads are app-proxied through the process (multipart → S3 `PutObject`); the body streams from the seekable `multipart.File`, so raising the limit further is safe up to S3's 5 GB single-PutObject ceiling, beyond which multipart/presigned upload would be needed.
