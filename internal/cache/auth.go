package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrPendingNotFound is returned by the attempt-increment scripts when the
// pending object they target has expired or was never created — e.g. the
// caller's 15-minute window (ADR-8) elapsed between reading and writing it.
var ErrPendingNotFound = errors.New("pending object not found or expired")

// PendingReg is the unfinished-registration (or password-reset) object kept
// in Redis only (ADR-8) — no row in `users` until verify succeeds. Both the
// password and the emailed code are stored as hashes; the plaintext code
// exists only between generation and being handed to the Mailer.
type PendingReg struct {
	PasswordHash string `json:"password_hash"`
	CodeHash     string `json:"code_hash"`
	Attempts     int    `json:"attempts"`
	Username     string `json:"username"`
}

// RedisAuthStore is the Redis-backed source of truth for auth flow state that
// is never a "cache" of anything else: pending registration/reset objects,
// the login failed-attempt counter, and the mail resend cooldown/quota. It
// works directly against *redis.Client (mirrors RedisPresenceStore), using
// Lua scripts where a check-then-act race would otherwise matter.
//
// Best-effort vs must-succeed: every method here returns its Redis error to
// the caller (none are "best-effort, log and continue" like RedisCache).
// Unlike a cache miss that safely degrades to a DB load, a lost pending
// registration, a failed attempt-counter increment, or a failed login-fail
// increment all have direct security consequences (an attacker who can make
// these calls silently no-op gets unlimited code guesses / unlimited login
// attempts / unlimited resend emails). So a Redis failure here must fail the
// request rather than silently allow it through.
type RedisAuthStore struct {
	rdb *redis.Client
	log *slog.Logger
}

// NewRedisAuthStore builds an auth state store over a Redis client.
func NewRedisAuthStore(rdb *redis.Client, log *slog.Logger) *RedisAuthStore {
	return &RedisAuthStore{rdb: rdb, log: log}
}

// --- pending registration -------------------------------------------------

// SavePendingReg overwrites the pending registration object for an email. A
// repeated register on the same email is a plain SET, not a read-modify-write
// — that's the whole point of ADR-8 (no DB row to clean up, no risk of
// squatting on the email UNIQUE constraint).
func (s *RedisAuthStore) SavePendingReg(ctx context.Context, email string, data PendingReg, ttl time.Duration) error {
	return s.savePending(ctx, PendingRegKey(email), data, ttl)
}

// GetPendingReg returns the pending registration object for an email.
// found=false means no pending registration (never started or expired).
func (s *RedisAuthStore) GetPendingReg(ctx context.Context, email string) (PendingReg, bool, error) {
	return s.getPending(ctx, PendingRegKey(email))
}

// DeletePendingReg removes the pending registration object (called once
// verify succeeds).
func (s *RedisAuthStore) DeletePendingReg(ctx context.Context, email string) error {
	return s.deletePending(ctx, PendingRegKey(email))
}

// IncrementPendingRegAttempts atomically increments the Attempts field of the
// pending registration object (ADR-8's max-attempts check on verify),
// preserving the object's remaining TTL — a failed attempt must not reset the
// 15-minute clock. Returns ErrPendingNotFound if the object has expired.
func (s *RedisAuthStore) IncrementPendingRegAttempts(ctx context.Context, email string) (int, error) {
	return s.incrementPendingAttempts(ctx, PendingRegKey(email))
}

// --- pending password reset ------------------------------------------------

// SavePendingReset overwrites the pending password-reset object for an
// email. Same overwrite-on-repeat semantics as SavePendingReg.
func (s *RedisAuthStore) SavePendingReset(ctx context.Context, email string, data PendingReg, ttl time.Duration) error {
	return s.savePending(ctx, PendingResetKey(email), data, ttl)
}

// GetPendingReset returns the pending password-reset object for an email.
func (s *RedisAuthStore) GetPendingReset(ctx context.Context, email string) (PendingReg, bool, error) {
	return s.getPending(ctx, PendingResetKey(email))
}

// DeletePendingReset removes the pending password-reset object (called once
// the reset succeeds).
func (s *RedisAuthStore) DeletePendingReset(ctx context.Context, email string) error {
	return s.deletePending(ctx, PendingResetKey(email))
}

// IncrementPendingResetAttempts is IncrementPendingRegAttempts for the
// password-reset flow (same max-attempts guard on the reset code).
func (s *RedisAuthStore) IncrementPendingResetAttempts(ctx context.Context, email string) (int, error) {
	return s.incrementPendingAttempts(ctx, PendingResetKey(email))
}

// --- shared pending-object helpers ------------------------------------------

func (s *RedisAuthStore) savePending(ctx context.Context, key string, data PendingReg, ttl time.Duration) error {
	encoded, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("auth store: encoding pending object: %w", err)
	}
	if err := s.rdb.Set(ctx, key, encoded, ttl).Err(); err != nil {
		return fmt.Errorf("auth store: saving pending object: %w", err)
	}
	return nil
}

func (s *RedisAuthStore) getPending(ctx context.Context, key string) (PendingReg, bool, error) {
	raw, err := s.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return PendingReg{}, false, nil
	}
	if err != nil {
		return PendingReg{}, false, fmt.Errorf("auth store: reading pending object: %w", err)
	}
	var data PendingReg
	if err := json.Unmarshal(raw, &data); err != nil {
		return PendingReg{}, false, fmt.Errorf("auth store: decoding pending object: %w", err)
	}
	return data, true, nil
}

func (s *RedisAuthStore) deletePending(ctx context.Context, key string) error {
	if err := s.rdb.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("auth store: deleting pending object: %w", err)
	}
	return nil
}

// incrementAttemptsScript atomically increments the "attempts" field of the
// JSON object at KEYS[1], preserving its remaining TTL (PTTL/PEXPIRE round
// trip so a rewrite never resets the 15-minute clock). Returns the new
// attempts count, or -1 if the key does not exist (expired/never created).
var incrementAttemptsScript = redis.NewScript(`
local raw = redis.call('GET', KEYS[1])
if not raw then
	return -1
end
local ttl = redis.call('PTTL', KEYS[1])
local data = cjson.decode(raw)
data.attempts = data.attempts + 1
local encoded = cjson.encode(data)
if ttl and ttl > 0 then
	redis.call('SET', KEYS[1], encoded, 'PX', ttl)
else
	redis.call('SET', KEYS[1], encoded)
end
return data.attempts
`)

func (s *RedisAuthStore) incrementPendingAttempts(ctx context.Context, key string) (int, error) {
	res, err := incrementAttemptsScript.Run(ctx, s.rdb, []string{key}).Int()
	if err != nil {
		return 0, fmt.Errorf("auth store: incrementing pending attempts: %w", err)
	}
	if res < 0 {
		return 0, ErrPendingNotFound
	}
	return res, nil
}

// --- login failure counter (ADR-13) ----------------------------------------

// incrementLoginFailScript atomically INCRs the counter and (only on the
// first increment of a window) sets its TTL, so two concurrent failed logins
// can't race the TTL-setting and leave the key immortal or reset the window.
// KEYS[1]=counter key, ARGV[1]=window ms. Returns the new count.
var incrementLoginFailScript = redis.NewScript(`
local n = redis.call('INCR', KEYS[1])
if n == 1 then
	redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return n
`)

// IncrementLoginFail atomically increments the failed-login counter for a
// login (email or username, lower-cased by LoginFailKey) within a sliding
// window and returns the new count. Threshold/window policy lives in the
// caller (service layer); this just does the atomic counting.
func (s *RedisAuthStore) IncrementLoginFail(ctx context.Context, login string, window time.Duration) (int, error) {
	res, err := incrementLoginFailScript.Run(ctx, s.rdb, []string{LoginFailKey(login)}, window.Milliseconds()).Int()
	if err != nil {
		return 0, fmt.Errorf("auth store: incrementing login fail counter: %w", err)
	}
	return res, nil
}

// ResetLoginFail clears the failed-login counter (called on successful
// login). Deleting a missing key is a no-op, not an error.
func (s *RedisAuthStore) ResetLoginFail(ctx context.Context, login string) error {
	if err := s.rdb.Del(ctx, LoginFailKey(login)).Err(); err != nil {
		return fmt.Errorf("auth store: resetting login fail counter: %w", err)
	}
	return nil
}

// --- mail resend cooldown / hourly quota ------------------------------------

// cooldownScript atomically claims a cooldown window: sets the key only if
// absent (NX) with a TTL, so a double-click on "resend" can't send two
// emails. KEYS[1]=cooldown key, ARGV[1]=ttl ms. Returns 1 if claimed
// (allowed), 0 if a cooldown is already active.
var cooldownScript = redis.NewScript(`
if redis.call('SET', KEYS[1], '1', 'PX', ARGV[1], 'NX') then
	return 1
else
	return 0
end
`)

// CheckAndSetMailCooldown atomically checks whether a resend is allowed and,
// if so, claims the cooldown window for the given duration. allowed=false
// means a cooldown is already active (a resend/double-click must be
// rejected without sending another email).
func (s *RedisAuthStore) CheckAndSetMailCooldown(ctx context.Context, email string, cooldown time.Duration) (bool, error) {
	res, err := cooldownScript.Run(ctx, s.rdb, []string{MailCooldownKey(email)}, cooldown.Milliseconds()).Int()
	if err != nil {
		return false, fmt.Errorf("auth store: checking mail cooldown: %w", err)
	}
	return res == 1, nil
}

// IncrementMailQuota atomically increments the hourly send-quota counter for
// an email and returns the new count (same INCR+PEXPIRE-on-first-increment
// pattern as IncrementLoginFail). The caller compares the result against the
// configured quota.
func (s *RedisAuthStore) IncrementMailQuota(ctx context.Context, email string, window time.Duration) (int, error) {
	res, err := incrementLoginFailScript.Run(ctx, s.rdb, []string{MailQuotaKey(email)}, window.Milliseconds()).Int()
	if err != nil {
		return 0, fmt.Errorf("auth store: incrementing mail quota: %w", err)
	}
	return res, nil
}

// --- password-reset mail cooldown / hourly quota ---------------------------
//
// Separate key namespace from CheckAndSetMailCooldown/IncrementMailQuota
// above (MailResetCooldownKey/MailResetQuotaKey vs MailCooldownKey/
// MailQuotaKey): ForgotPassword only claims these on the found-account path,
// and sharing a namespace with ResendCode (which reports a hit as a visible
// 429) would let two calls on the same email deterministically reveal
// whether the account exists. See task-6 report, Critical #1.

// CheckAndSetMailResetCooldown is CheckAndSetMailCooldown for the
// password-reset flow.
func (s *RedisAuthStore) CheckAndSetMailResetCooldown(ctx context.Context, email string, cooldown time.Duration) (bool, error) {
	res, err := cooldownScript.Run(ctx, s.rdb, []string{MailResetCooldownKey(email)}, cooldown.Milliseconds()).Int()
	if err != nil {
		return false, fmt.Errorf("auth store: checking reset mail cooldown: %w", err)
	}
	return res == 1, nil
}

// IncrementMailResetQuota is IncrementMailQuota for the password-reset flow.
func (s *RedisAuthStore) IncrementMailResetQuota(ctx context.Context, email string, window time.Duration) (int, error) {
	res, err := incrementLoginFailScript.Run(ctx, s.rdb, []string{MailResetQuotaKey(email)}, window.Milliseconds()).Int()
	if err != nil {
		return 0, fmt.Errorf("auth store: incrementing reset mail quota: %w", err)
	}
	return res, nil
}

// --- ResetPassword wrong-code attempt counter -------------------------------
//
// Same INCR+PEXPIRE-on-first-increment pattern as IncrementLoginFail
// (ADR-13), reused here for the same reason: the counter must exist and be
// incremented UNCONDITIONALLY, regardless of whether a real PendingReset
// object backs `email`. If this only counted attempts against a real
// pending object (as IncrementPendingResetAttempts alone did), a
// nonexistent email would always get 400 forever while a real one
// eventually hit 429 — an account-existence oracle via ResetPassword's own
// status codes. See task-6 report, fix round 2.

// IncrementResetAttempts atomically increments the wrong-code counter for a
// password-reset attempt on email and returns the new count. Unlike
// IncrementPendingResetAttempts (which requires a live pending object and
// preserves ITS remaining TTL), this is a plain sliding-window counter that
// exists independently — window is set on the FIRST increment only, same
// as IncrementLoginFail, so it doesn't require any other Redis object to
// exist first.
func (s *RedisAuthStore) IncrementResetAttempts(ctx context.Context, email string, window time.Duration) (int, error) {
	res, err := incrementLoginFailScript.Run(ctx, s.rdb, []string{ResetAttemptsKey(email)}, window.Milliseconds()).Int()
	if err != nil {
		return 0, fmt.Errorf("auth store: incrementing reset attempt counter: %w", err)
	}
	return res, nil
}

// ResetResetAttempts clears the wrong-code counter (called once a reset
// succeeds, or once the counter is exhausted and the caller must start
// over). Deleting a missing key is a no-op, not an error.
func (s *RedisAuthStore) ResetResetAttempts(ctx context.Context, email string) error {
	if err := s.rdb.Del(ctx, ResetAttemptsKey(email)).Err(); err != nil {
		return fmt.Errorf("auth store: resetting reset attempt counter: %w", err)
	}
	return nil
}
