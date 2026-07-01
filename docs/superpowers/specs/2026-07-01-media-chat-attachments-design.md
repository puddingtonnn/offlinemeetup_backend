# Media chat attachments (photo / video / audio), up to 100 MB

**Date:** 2026-07-01
**Status:** Approved (design)

## Goal

Allow chat messages to carry **media** attachments — photo, video, and audio —
instead of images only. Size limit up to **100 MB** (WhatsApp/Signal class).
Meetup covers and profile avatars stay **image-only**.

## Context (current state)

- A single endpoint `POST /v1/files/upload` (`FileHandler.Upload` →
  `FileService.Upload`) backs **three** references: chat `messages.file_id`,
  meetup `cover_file_id`, profile `avatar_file_id`. At reference time the repo
  only checks **ownership** (`fileOwnedBy`), never type.
- `FileService.Upload` today: rejects anything whose sniffed
  (`http.DetectContentType`) type is not in a 4-entry image whitelist; derives
  both the stored `Content-Type` and the object-key extension from the **sniffed**
  type (never the client filename) — this is a security invariant (no
  active-content extension in a public S3 key; no bytes disguised as an image).
- Size is hardcoded `10 << 20` in **three** places: `MaxBytesReader` and
  `ParseMultipartForm` in the handler, `maxFileSize` const in the service.
- The message DTO is already generic: `message_type="file"` +
  `Attachment{url,file_name,mime_type,size}`; the client renders by `mime_type`.
  A soft-deleted message hides its attachment. **No DTO change needed.**
- `BodyLimit(1MB)` global middleware **skips** `multipart/` requests, so it does
  not cap uploads.
- `http.Server` sets **no** `ReadTimeout`/`WriteTimeout`, so a slow large upload
  is not cut off.
- S3 client is aws-sdk-go-v2 with `UsePathStyle` (MinIO); the service uses raw
  `PutObject`. The body is currently `io.MultiReader(bytes.NewReader(head),
  reader)` — **non-seekable**, which forces the SDK to buffer the whole payload
  in RAM to compute its signature.

## Decisions

- **Types:** media only — photo, video, audio. No documents, no archives.
- **Size:** up to 100 MB, configurable.
- **Architecture:** keep the current app-proxied multipart upload (no
  presigned/direct-to-S3). 100 MB through the process is fine when we stream a
  seekable body.
- **Covers/avatars stay image-only**, enforced at reference time.
- **Validation stays byte-based** (sniff), never filename-based.

## Type policy

Allowed (canonical MIME → stored extension):

- **Images:** `image/jpeg`→`.jpg`, `image/png`→`.png`, `image/webp`→`.webp`,
  `image/gif`→`.gif`, `image/heic`→`.heic`, `image/heif`→`.heif`.
- **Video:** `video/mp4`→`.mp4`, `video/quicktime`→`.mov`, `video/webm`→`.webm`,
  `video/x-matroska`→`.mkv`, `video/3gpp`→`.3gp`, `video/x-msvideo`→`.avi`.
- **Audio:** `audio/mpeg`→`.mp3`, `audio/mp4`→`.m4a`, `audio/aac`→`.aac`,
  `audio/ogg`→`.ogg`, `audio/wav`→`.wav`, `audio/flac`→`.flac`.

Explicitly **rejected**: `image/svg+xml` (active-content / stored XSS), all
documents/archives/executables, anything not resolvable to a whitelisted media
type.

## Detection strategy (the main fork)

Go's `http.DetectContentType` misses common phone media (`.mov` QuickTime brand,
`.m4a`, often `.mkv`, `heic`) — they sniff to `application/octet-stream`. So:

New pure package `internal/service/media`:

```
Detect(head []byte) (mime string, ext string, ok bool)
```

1. Run `http.DetectContentType(head)`; if the result is a whitelisted media type,
   map it to the canonical extension and return.
2. Otherwise consult a small supplementary magic-byte table for media Go misses:
   - ISOBMFF `....ftyp` box → inspect the brand to split video (`mp4`/`mov`/`3gp`),
     audio (`M4A `→`audio/mp4`), and images (`heic`/`heif`/`mif1`→`image/heic`).
   - EBML `1A 45 DF A3` → `video/webm` (mkv/webm).
   - `OggS` → `audio/ogg`; `fLaC` → `audio/flac`; `RIFF....WAVE` → `audio/wav`;
     `RIFF....AVI ` → `video/x-msvideo`; ADTS/AAC sync → `audio/aac`.
3. Unknown → `ok=false` (rejected).

Canonical MIME + extension always come from `Detect`, never from the client
filename — preserving the existing security invariant.

## Components / changes

1. **`internal/service/media/detect.go`** (new) — `Detect` + magic-byte table.
   Pure, table-tested.
2. **`internal/service/file.go`** — `Upload` accepts `io.ReadSeeker` (the
   `multipart.File`). Read 512-byte head → `media.Detect` → `Seek(0,0)` →
   `PutObject{Body: seekable, ContentLength: size, ContentType: detected}`.
   Remove `allowedImageTypes`, `imageTypeExt`, `maxFileSize`.
3. **`internal/config/config.go`** — add `MaxUploadSize int64` from
   `MAX_UPLOAD_SIZE` (default `100 << 20`). Optional env (has a default), so
   **not** a fail-fast secret.
4. **`internal/transport/http/handler/file.go`** — `MaxBytesReader(w, body,
   cfg.MaxUploadSize)`; `ParseMultipartForm(~16<<20)` (spill beyond that to a
   temp file, which is seekable); pass the seekable `multipart.File` to the
   service. Update Swagger note.
5. **Cover/avatar image gate (reference time).** New sentinel
   `repo.ErrFileNotImage` and a helper `imageFileOwnedBy(ctx, idb, fileID,
   userID)` both live in `repo/file.go` (next to `fileOwnedBy`/`ErrFileNotOwned`);
   the helper checks ownership **and** `mime_type LIKE 'image/%'`, returning
   `ErrFileNotOwned` when not owned and `ErrFileNotImage` when owned-but-not-image.
   The cover paths (`repo/meetup.go` create + update) and the avatar path
   (`repo/profile.go`) call it instead of `fileOwnedBy`. The service boundary maps
   `ErrFileNotImage` to `service.ErrInvalidInput` (400) in `mapMeetupRepoError`
   and the inline profile translation. Chat attachments are **not** gated (any
   media allowed).
6. **DTO — unchanged.** `message_type` stays `"file"`; client uses `mime_type`.

## Data flow (chat attachment)

`POST /v1/files/upload` (media) → `Detect` → S3 + `files` row (uploaded_by=caller)
→ client puts returned `id` into the send payload → `SendMessage(fileID)` →
`repo.SaveMessage` enforces `fileOwnedBy` in-tx → broadcast → DTO builds
`Attachment` with `publicURL`.

## Error handling

- Unsupported/undetectable type → `ErrInvalidInput` (400).
- Size out of range (`size <= 0 || size > MaxUploadSize`) → `ErrInvalidInput`;
  `MaxBytesReader` also hard-caps the stream.
- Non-image referenced as cover/avatar → `ErrFileNotImage` → `ErrInvalidInput`
  (400).
- Referencing someone else's file → `ErrFileNotOwned` → `ErrForbidden` (403),
  unchanged.

## Testing

- `media.Detect`: table tests per type via magic-byte fixtures (incl.
  mov/m4a/heic/webm/mkv/ogg/flac/wav); reject svg, pdf, exe, empty.
- `FileService.Upload`: accept a video and an audio; reject a document and svg;
  size limit read from config; verify the seek-then-put path.
- Repo: cover/avatar reject a non-image (`ErrFileNotImage`) and accept an image;
  chat attachment accepts a video.
- `config`: `MAX_UPLOAD_SIZE` parsed; default applied when unset (keep
  `setRequiredSecrets` helper; new var is optional).

## Non-goals (YAGNI)

Presigned / direct-to-S3 uploads; server-side transcoding or thumbnail/preview
generation; per-type size limits; a finer `message_type` (image/video/audio);
duration/dimension extraction.

## Touch list

- `internal/service/media/detect.go` (new) + test
- `internal/service/file.go` + test
- `internal/config/config.go` + test
- `internal/transport/http/handler/file.go`
- `internal/repo/file.go` — `imageFileOwnedBy` helper + `ErrFileNotImage`
- `internal/repo/meetup.go`, `internal/repo/profile.go` — use the gate
- `internal/service/meetup.go` (`mapMeetupRepoError`), `internal/service/profile.go` — map sentinel
- `CLAUDE.md` — update the "allowed image" security note; `.env`/`.env.example` — `MAX_UPLOAD_SIZE`
