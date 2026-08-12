package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/cache"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/dto"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/repo"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/safego"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/service/mail"
)

// bcryptCost is fixed at 12 per ADR-12.
const bcryptCost = 12

// PasswordUserRepository is the slice of the user repository the email/password
// registration flow needs. Declared at the consumer (CLAUDE.md), implemented by
// *repo.UserRepo.
type PasswordUserRepository interface {
	FindIDByEmail(ctx context.Context, email string) (int64, error)
	FindIDByUsername(ctx context.Context, username string) (int64, error)
	CreateUserWithPassword(ctx context.Context, email, username, passwordHash string) (int64, error)
	AttachPassword(ctx context.Context, userID int64, passwordHash string) error
}

// AuthStore is the Redis-backed auth state this flow needs: the pending
// registration object (ADR-8), the mail cooldown/quota throttles, and (used
// by Login in auth_login.go) the failed-login counter (ADR-13). Implemented
// by *cache.RedisAuthStore.
type AuthStore interface {
	SavePendingReg(ctx context.Context, email, regID string, data cache.PendingReg, ttl time.Duration) error
	GetPendingReg(ctx context.Context, email, regID string) (cache.PendingReg, bool, error)
	DeletePendingReg(ctx context.Context, email, regID string) error
	IncrementPendingRegAttempts(ctx context.Context, email, regID string) (int, error)
	CheckAndSetMailCooldown(ctx context.Context, email string, cooldown time.Duration) (bool, error)
	IncrementMailQuota(ctx context.Context, email string, window time.Duration) (int, error)
	IncrementLoginFail(ctx context.Context, login string, window time.Duration) (int, error)
	ResetLoginFail(ctx context.Context, login string) error
	// Pending password reset (Task 6) — same shape/lifecycle as pending
	// registration above, separate Redis key space (cache.PendingResetKey).
	SavePendingReset(ctx context.Context, email string, data cache.PendingReg, ttl time.Duration) error
	GetPendingReset(ctx context.Context, email string) (cache.PendingReg, bool, error)
	DeletePendingReset(ctx context.Context, email string) error
	// Reset-flow mail cooldown/quota (fix round 1, Critical #1): a SEPARATE
	// key namespace from CheckAndSetMailCooldown/IncrementMailQuota above,
	// so ForgotPassword's conditional (found-only) claim can never be
	// observed through ResendCode's visible 429 on the same key.
	CheckAndSetMailResetCooldown(ctx context.Context, email string, cooldown time.Duration) (bool, error)
	IncrementMailResetQuota(ctx context.Context, email string, window time.Duration) (int, error)
	// Reset-flow wrong-code attempt counter (fix round 2): maintained
	// UNCONDITIONALLY per email, independent of whether a real
	// PendingReset object exists — see ResetPassword/rejectResetAttempt.
	// Replaces IncrementPendingResetAttempts (Task 3), which required a
	// live pending object and so couldn't give a nonexistent email the
	// same eventual-429 shape a real account gets.
	IncrementResetAttempts(ctx context.Context, email string, window time.Duration) (int, error)
	ResetResetAttempts(ctx context.Context, email string) error
}

// Mailer sends a plain-text email. Consumer-side declaration of
// mail.Mailer so this package doesn't depend on the concrete implementation.
type Mailer interface {
	Send(ctx context.Context, to, subject, body string) error
}

// normalizeEmail applies ADR-3: emails are stored and keyed lower-cased and
// trimmed, so `Bob@Example.com ` and `bob@example.com` are one account.
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// generateCode returns a 6-digit confirmation code from crypto/rand. Six
// digits is an SMS-style UX; brute force is bounded by EMAIL_CODE_MAX_ATTEMPTS
// and the 15-minute TTL, not by the code's entropy.
func generateCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", fmt.Errorf("generating confirmation code: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// hashCode hashes a confirmation code for storage. The plaintext code exists
// only between generation and being handed to the Mailer; only this hash is
// persisted (see the concerns note in the task report: a 6-digit code is
// trivially brute-forced offline, so this protects against incidental
// exposure — logs, a dump — not against an attacker who already reads Redis,
// who could simply overwrite the object).
func hashCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

func codeMatches(code, storedHash string) bool {
	return subtle.ConstantTimeCompare([]byte(hashCode(code)), []byte(storedHash)) == 1
}

// newRegistrationID returns the opaque identifier of ONE registration attempt.
// /register hands it to the caller and /verify-email must send it back; it is
// half of the Redis key holding the pending object (cache.PendingRegKey).
//
// Unlike the emailed code it is not a secret and proves nothing — it only
// separates concurrent attempts on the same email so one can't overwrite
// another. 128 bits of CSPRNG is therefore about collision-freedom, not
// guessing resistance: two attempts must never land on the same key.
func newRegistrationID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating registration id: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

// Register starts an email/password registration (ADR-1, ADR-8) and returns
// the attempt's registration ID, which /verify-email must send back. Nothing
// is written to Postgres: the password hash, the code hash and the chosen
// username live in Redis for EMAIL_CODE_TTL under the (email, regID) pair.
//
// The response is identical whether or not the email already has an account
// (ADR-7) — only the email template differs — so the endpoint is not an
// account-existence oracle. That symmetry is why the registration ID is
// generated FIRST and returned on every non-error path, including the one
// where the hourly quota silently suppresses the send: a caller must not be
// able to tell "email sent" from "email suppressed" by the response shape.
func (s *AuthService) Register(ctx context.Context, email, username, password string) (string, error) {
	email = normalizeEmail(email)
	username = strings.TrimSpace(username)

	if !dto.ValidEmail(email) {
		return "", fmt.Errorf("%w: invalid email", ErrInvalidInput)
	}
	if !dto.ValidUsername(username) {
		return "", fmt.Errorf("%w: invalid username", ErrInvalidInput)
	}
	if !dto.ValidPassword(password) {
		return "", fmt.Errorf("%w: invalid password", ErrInvalidInput)
	}

	regID, err := newRegistrationID()
	if err != nil {
		return "", err
	}

	// Soft (non-authoritative) username check: a fast 400 so we don't email a
	// registration that is already doomed. The unique index checked inside
	// VerifyEmail's transaction is the real arbiter (ADR-9).
	switch _, err := s.passwordRepo.FindIDByUsername(ctx, username); {
	case err == nil:
		return "", fmt.Errorf("%w: username already taken", ErrInvalidInput)
	case errors.Is(err, repo.ErrNotFound):
		// free (for now)
	default:
		return "", err
	}

	// The hourly quota is checked before the send, and is what bounds the
	// number of pending objects an attacker can pile onto one email — since
	// regID makes every /register create its OWN key, the per-email quota is
	// the only thing keeping that from being unbounded.
	if !s.reserveMailQuota(ctx, email) {
		s.log.Warn("registration email suppressed by hourly quota", slog.String("email", email))
		return regID, nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("hashing password: %w", err)
	}

	code, err := generateCode()
	if err != nil {
		return "", err
	}

	// ADR-7: whether the email already has an account changes only which
	// template we send, never the status code or body shape.
	accountExists := false
	switch _, err := s.passwordRepo.FindIDByEmail(ctx, email); {
	case err == nil:
		accountExists = true
	case errors.Is(err, repo.ErrNotFound):
	default:
		return "", err
	}

	pending := cache.PendingReg{
		PasswordHash: string(hash),
		CodeHash:     hashCode(code),
		Attempts:     0,
		Username:     username,
	}
	if err := s.authStore.SavePendingReg(ctx, email, regID, pending, s.cfg.EmailCodeTTL); err != nil {
		return "", err
	}

	var subject, body string
	if accountExists {
		subject, body = mail.ExistingAccountVerification(code, s.cfg.EmailCodeTTL)
	} else {
		subject, body = mail.RegistrationVerification(username, code, s.cfg.EmailCodeTTL)
	}
	s.sendMailAsync(ctx, email, subject, body)

	return regID, nil
}

// VerifyEmail confirms a pending registration and logs the caller in.
//
// regID is the registration ID /register returned; together with email it
// addresses exactly one registration attempt. A wrong or unknown pair is
// reported the same way as an expired one — there is nothing to distinguish.
//
// chosenUsername is the ADR-9 retry path: if the username picked at register
// time was taken in the meantime, verify returns 409 WITHOUT destroying the
// pending object, and the caller resubmits with a different username.
func (s *AuthService) VerifyEmail(ctx context.Context, email, regID, code string, chosenUsername *string) (*TokenPair, error) {
	email = normalizeEmail(email)

	pending, found, err := s.authStore.GetPendingReg(ctx, email, regID)
	if err != nil {
		return nil, err
	}
	if !found {
		// Never started, wrong email/regID pair, or the 15-minute window
		// elapsed.
		return nil, fmt.Errorf("%w: no pending registration", ErrInvalidInput)
	}

	if pending.Attempts >= s.cfg.EmailCodeMaxAttempts {
		s.dropPending(ctx, email, regID)
		return nil, fmt.Errorf("%w: too many invalid codes", ErrTooManyRequests)
	}

	if !codeMatches(code, pending.CodeHash) {
		attempts, err := s.authStore.IncrementPendingRegAttempts(ctx, email, regID)
		if err != nil {
			if errors.Is(err, cache.ErrPendingNotFound) {
				return nil, fmt.Errorf("%w: no pending registration", ErrInvalidInput)
			}
			return nil, err
		}
		if attempts >= s.cfg.EmailCodeMaxAttempts {
			// Burned through the allowance: invalidate the pending object so the
			// remaining guesses can't be spent, and make the caller start over.
			s.dropPending(ctx, email, regID)
			return nil, fmt.Errorf("%w: too many invalid codes", ErrTooManyRequests)
		}
		return nil, fmt.Errorf("%w: invalid code", ErrInvalidInput)
	}

	username := pending.Username
	if chosenUsername != nil && strings.TrimSpace(*chosenUsername) != "" {
		username = strings.TrimSpace(*chosenUsername)
		if !dto.ValidUsername(username) {
			return nil, fmt.Errorf("%w: invalid username", ErrInvalidInput)
		}
	}

	userID, err := s.commitRegistration(ctx, email, username, pending.PasswordHash)
	if err != nil {
		return nil, err
	}

	// Only now is the pending object spent. A failure here is logged, not
	// fatal: the object expires by TTL anyway, and a replayed verify would
	// take the ADR-7 attach path (an idempotent credentials upsert).
	if err := s.authStore.DeletePendingReg(ctx, email, regID); err != nil {
		s.log.Error("deleting spent pending registration", slog.String("err", err.Error()))
	}

	return s.issueTokenPair(ctx, userID)
}

// commitRegistration turns a confirmed pending registration into an account.
// The "does this email already have an account" question (ADR-7) is answered
// HERE, freshly, rather than being carried in the Redis pending object — see
// the task report: the pending object can be up to 15 minutes stale, during
// which a Google/Telegram login could have created the account, and a stale
// "no account" answer would send us down the create path straight into the
// users-email unique index.
func (s *AuthService) commitRegistration(ctx context.Context, email, username, passwordHash string) (int64, error) {
	existingID, err := s.passwordRepo.FindIDByEmail(ctx, email)
	switch {
	case err == nil:
		// ADR-7: attach a password to the existing account. Its profile (and
		// username) is left untouched; the username submitted at register time
		// is only used when we actually create a new account.
		if err := s.passwordRepo.AttachPassword(ctx, existingID, passwordHash); err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				return 0, ErrNotFound
			}
			return 0, err
		}
		// AttachPassword is an upsert: if the account already had a password,
		// this just replaced it. Any password-hash mutation must kill every
		// existing session, otherwise a stolen refresh token outlives the
		// password it was obtained under (same rule reset/change-password
		// follow). Revoke BEFORE issueTokenPair so the caller's own fresh pair
		// — minted after this returns — survives.
		if err := s.refreshRepo.RevokeAllForUser(ctx, existingID); err != nil {
			return 0, fmt.Errorf("revoking sessions after password attach: %w", err)
		}
		return existingID, nil

	case errors.Is(err, repo.ErrNotFound):
		userID, err := s.passwordRepo.CreateUserWithPassword(ctx, email, username, passwordHash)
		if err != nil {
			// ADR-9: username lost the race. 409, and the caller retries verify
			// with a different username — the pending object stays alive.
			if errors.Is(err, repo.ErrUsernameTaken) {
				return 0, fmt.Errorf("%w: username already taken", ErrAlreadyExists)
			}
			// The email gained an account between the lookup above and this
			// insert. Also non-destructive: retrying verify now takes the
			// attach branch.
			if errors.Is(err, repo.ErrEmailTaken) {
				return 0, fmt.Errorf("%w: email already registered", ErrAlreadyExists)
			}
			return 0, err
		}
		return userID, nil

	default:
		return 0, err
	}
}

// ResendCode issues a fresh confirmation code for one pending registration,
// addressed by the same (email, regID) pair as VerifyEmail.
//
// It always reports success, including when there is no pending registration
// for the pair: the plan's API table lists only 202/429 for this endpoint, and
// a 400-on-unknown-email would turn resend into the account-enumeration oracle
// that ADR-7 spends the whole register flow avoiding. The throttles are applied
// BEFORE the pending lookup so a prober can't distinguish the two cases by
// which limits get consumed either.
func (s *AuthService) ResendCode(ctx context.Context, email, regID string) error {
	email = normalizeEmail(email)
	if !dto.ValidEmail(email) {
		return fmt.Errorf("%w: invalid email", ErrInvalidInput)
	}

	allowed, err := s.authStore.CheckAndSetMailCooldown(ctx, email, s.cfg.EmailResendCooldown)
	if err != nil {
		return err
	}
	if !allowed {
		return fmt.Errorf("%w: resend cooldown active", ErrTooManyRequests)
	}

	sent, err := s.authStore.IncrementMailQuota(ctx, email, time.Hour)
	if err != nil {
		return err
	}
	if sent > s.cfg.EmailSendQuotaPerHour {
		return fmt.Errorf("%w: hourly email quota exceeded", ErrTooManyRequests)
	}

	pending, found, err := s.authStore.GetPendingReg(ctx, email, regID)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}

	code, err := generateCode()
	if err != nil {
		return err
	}

	// A resend restarts the window: fresh code, fresh TTL, attempts back to 0
	// (the previous code is dead, so the guesses spent against it are moot).
	// This is the one place a pending object is legitimately rewritten — and
	// it rewrites the caller's OWN attempt, since regID had to be presented
	// to find it in the first place.
	pending.CodeHash = hashCode(code)
	pending.Attempts = 0
	if err := s.authStore.SavePendingReg(ctx, email, regID, pending, s.cfg.EmailCodeTTL); err != nil {
		return err
	}

	accountExists := false
	switch _, err := s.passwordRepo.FindIDByEmail(ctx, email); {
	case err == nil:
		accountExists = true
	case errors.Is(err, repo.ErrNotFound):
	default:
		return err
	}

	var subject, body string
	if accountExists {
		subject, body = mail.ExistingAccountVerification(code, s.cfg.EmailCodeTTL)
	} else {
		subject, body = mail.RegistrationVerification(pending.Username, code, s.cfg.EmailCodeTTL)
	}
	s.sendMailAsync(ctx, email, subject, body)

	return nil
}

// reserveMailQuota consumes one unit of the per-email hourly send quota and
// reports whether the send is allowed. A Redis failure is logged and treated
// as "allowed": this is spam control layered on top of the IP rate limiter,
// and failing a registration because the quota counter is unreachable would be
// worse than letting one extra email through.
func (s *AuthService) reserveMailQuota(ctx context.Context, email string) bool {
	sent, err := s.authStore.IncrementMailQuota(ctx, email, time.Hour)
	if err != nil {
		s.log.Error("incrementing mail quota", slog.String("err", err.Error()))
		return true
	}
	return sent <= s.cfg.EmailSendQuotaPerHour
}

// dropPending invalidates a pending registration, logging (not returning) a
// failure — every caller is already on an error path.
func (s *AuthService) dropPending(ctx context.Context, email, regID string) {
	if err := s.authStore.DeletePendingReg(ctx, email, regID); err != nil {
		s.log.Error("invalidating pending registration", slog.String("err", err.Error()))
	}
}

// ForgotPassword starts a password-reset flow (ADR-14). login is the same
// email-or-username field as Login (ADR-2), resolved the same way
// (isEmailLogin).
//
// This ALWAYS returns nil (→ 202), whether or not login resolves to a real
// account — the plan's "rules easy to lose" bullet: a different response
// (or a different set of side effects visible to a timing/enumeration
// attacker) for "no such account" would turn this endpoint into exactly the
// oracle ADR-7 spends the whole register flow avoiding.
//
// Fix round (Critical #2): everything past the initial login→userID lookup
// (GetByID, cooldown/quota checks, code generation, SavePendingReset, the
// send) runs in a BACKGROUND goroutine (forgotPasswordAsync), not before
// this function returns. Unlike Login, nothing in this flow pays a bcrypt
// cost, so there was no ~300ms floor to hide the found path's extra 2
// Postgres + 2-3 Redis round trips under — that made the found/not-found
// timing difference a practically measurable oracle, not just a
// theoretical one. Now both branches cost exactly the one initial lookup
// before returning.
//
// The reset-flow cooldown/quota also moved to its OWN key namespace
// (cache.MailResetCooldownKey/MailResetQuotaKey via
// CheckAndSetMailResetCooldown/IncrementMailResetQuota) — Critical #1: the
// previous code shared ResendCode's cache.MailCooldownKey/MailQuotaKey, and
// since ForgotPassword only claims that key on the found-account path while
// ResendCode reports a hit on it as a visible 429, calling
// forgot-password then resend-code on the same email was a deterministic
// two-request account-existence oracle.
func (s *AuthService) ForgotPassword(ctx context.Context, login string) error {
	login = strings.TrimSpace(login)

	var (
		userID int64
		err    error
	)
	if isEmailLogin(login) {
		userID, err = s.passwordRepo.FindIDByEmail(ctx, normalizeEmail(login))
	} else {
		userID, err = s.passwordRepo.FindIDByUsername(ctx, login)
	}
	switch {
	case err == nil:
		// Found: the rest of the work happens off the request path (see
		// forgotPasswordAsync) so the synchronous cost here matches the
		// not-found branch exactly — one lookup, then return.
		s.forgotPasswordAsync(ctx, userID)
	case errors.Is(err, repo.ErrNotFound):
		// Unknown login: identical nil return, no further work at all.
	default:
		return err
	}

	return nil
}

// forgotPasswordAsync does the found-account work for ForgotPassword off the
// request goroutine: resolves the account's email, applies the reset-flow
// cooldown/quota, generates and saves the reset code, and sends the email.
// Every failure here is logged and simply abandons the background job — by
// the time this runs the HTTP response has already been sent (always nil/
// 202), so there is nothing left to propagate an error to, and no reason to
// make the caller wait on any of it.
func (s *AuthService) forgotPasswordAsync(ctx context.Context, userID int64) {
	bgCtx := context.WithoutCancel(ctx)
	log := s.log

	safego.Go(log, func() {
		user, err := s.repo.GetByID(bgCtx, userID)
		if err != nil {
			log.Error("forgot-password: loading user", slog.String("err", err.Error()))
			return
		}
		if user == nil {
			// Resolved an ID that vanished between the lookup and here.
			return
		}
		email := normalizeEmail(user.Email)
		if email == "" {
			// Telegram-only account with no email on file (findOrCreateUser
			// creates these with Email: "") — nothing to send to. Silent
			// no-op: the HTTP response was already a plain 202 either way,
			// so this can't leak anything, and touching a shared "" key
			// across every emailless user would be its own bug.
			return
		}

		if !s.checkResetMailCooldown(bgCtx, email) {
			return
		}
		if !s.reserveResetMailQuota(bgCtx, email) {
			return
		}

		code, err := generateCode()
		if err != nil {
			log.Error("forgot-password: generating code", slog.String("err", err.Error()))
			return
		}

		pending := cache.PendingReg{CodeHash: hashCode(code)}
		if err := s.authStore.SavePendingReset(bgCtx, email, pending, s.cfg.EmailCodeTTL); err != nil {
			log.Error("forgot-password: saving pending reset", slog.String("err", err.Error()))
			return
		}

		// Personalization only — not security-relevant, so the greeting is
		// left generic (empty name) rather than adding a Profile-lookup
		// dependency this service doesn't otherwise need.
		subject, body := mail.PasswordReset("", code, s.cfg.EmailCodeTTL)
		s.sendMailAsync(bgCtx, email, subject, body)
	})
}

// checkResetMailCooldown is the reset-flow's anti-double-send cooldown
// check, keyed separately from Register/ResendCode's cache.MailCooldownKey
// (Critical #1). Fail-open on a Redis error (logs, returns true/"allowed"):
// this call already runs off the request path, so a Redis hiccup here can
// only cost at most one duplicate email, never a wrong HTTP response.
func (s *AuthService) checkResetMailCooldown(ctx context.Context, email string) bool {
	allowed, err := s.authStore.CheckAndSetMailResetCooldown(ctx, email, s.cfg.EmailResendCooldown)
	if err != nil {
		s.log.Error("checking reset mail cooldown", slog.String("err", err.Error()))
		return true
	}
	return allowed
}

// reserveResetMailQuota is reserveMailQuota's counterpart for the
// password-reset flow, using the separate cache.MailResetQuotaKey namespace
// (Critical #1) instead of Register/ResendCode's cache.MailQuotaKey. Same
// fail-open reasoning as reserveMailQuota.
func (s *AuthService) reserveResetMailQuota(ctx context.Context, email string) bool {
	sent, err := s.authStore.IncrementMailResetQuota(ctx, email, time.Hour)
	if err != nil {
		s.log.Error("incrementing reset mail quota", slog.String("err", err.Error()))
		return true
	}
	return sent <= s.cfg.EmailSendQuotaPerHour
}

// dropPendingReset invalidates a pending password reset, logging (not
// returning) a failure — every caller is already on an error/terminal path.
func (s *AuthService) dropPendingReset(ctx context.Context, email string) {
	if err := s.authStore.DeletePendingReset(ctx, email); err != nil {
		s.log.Error("invalidating pending password reset", slog.String("err", err.Error()))
	}
}

// rejectResetAttempt handles a wrong reset code — or the absence of any
// pending-reset object at all (unknown email, or nobody ever called
// forgot-password for it). The two cases are DELIBERATELY handled by the
// exact same code path here: the wrong-attempt counter
// (cache.ResetAttemptsKey, via IncrementResetAttempts) is maintained
// unconditionally per email, whether or not a real PendingReset object
// backs it — mirroring ADR-13's IncrementLoginFail, a failed-login counter
// that likewise doesn't require the login to resolve to a real account.
//
// Fix round 2: before this, "no pending object" short-circuited straight to
// a plain 400 without ever touching a counter, so a nonexistent email
// always got 400 forever while a real account with the same number of
// wrong guesses eventually got 429 — an account-existence oracle via
// ResetPassword's own status codes (calling forgot-password then
// reset-password EmailCodeMaxAttempts times on the same email would reveal
// whether it existed). Now both cases increment the SAME counter and hit
// 429 on the SAME call count, and both do exactly one Redis round trip here
// (IncrementResetAttempts) on top of the one GetPendingReset ResetPassword
// already did — the real-pending and no-pending cases cost the identical
// number of round trips, so this doesn't reopen the round-trip-count timing
// gap Critical #2 closed for ForgotPassword.
func (s *AuthService) rejectResetAttempt(ctx context.Context, email string) error {
	attempts, err := s.authStore.IncrementResetAttempts(ctx, email, s.cfg.EmailCodeTTL)
	if err != nil {
		return err
	}
	if attempts >= s.cfg.EmailCodeMaxAttempts {
		// Burn the allowance: drop any real pending object (if one exists;
		// a no-op otherwise) and reset the counter so a subsequent attempt
		// starts a fresh cycle rather than being permanently stuck at 429 —
		// symmetric with VerifyEmail's dropPending behavior, and harmless
		// either way since a dropped pending object makes success
		// impossible regardless of further guesses.
		s.dropPendingReset(ctx, email)
		if err := s.authStore.ResetResetAttempts(ctx, email); err != nil {
			s.log.Error("resetting exhausted reset-attempt counter", slog.String("err", err.Error()))
		}
		return fmt.Errorf("%w: too many invalid codes", ErrTooManyRequests)
	}
	return fmt.Errorf("%w: invalid code", ErrInvalidInput)
}

// ResetPassword completes a forgot-password flow: confirms the emailed code
// and sets a new password. Same attempt-limiting SHAPE as VerifyEmail (Task
// 4) — 400 on a wrong code, 429 once attempts are exhausted — but the
// counter backing it is NOT tied to a real PendingReset object existing
// (fix round 2, see rejectResetAttempt): a nonexistent email and a real
// account with the same number of wrong attempts return the identical
// sequence of errors.
//
// This does NOT log the caller in — per the plan's API table, reset-password
// returns 200 (no AuthTokensResponse), it's a password reset, not a login —
// and it revokes every existing refresh token for the user ("rules easy to
// lose": a stolen session must not survive a password reset).
func (s *AuthService) ResetPassword(ctx context.Context, email, code, newPassword string) error {
	email = normalizeEmail(email)

	if !dto.ValidPassword(newPassword) {
		return fmt.Errorf("%w: invalid password", ErrInvalidInput)
	}

	pending, found, err := s.authStore.GetPendingReset(ctx, email)
	if err != nil {
		return err
	}
	if !found || !codeMatches(code, pending.CodeHash) {
		// Deliberately the SAME path for "no pending object at all" and
		// "pending object exists but the code is wrong" — see
		// rejectResetAttempt's doc comment for why.
		return s.rejectResetAttempt(ctx, email)
	}

	// Code confirmed: clear the wrong-attempt counter too (mirrors
	// ResetLoginFail clearing on a successful login) so a confirmed reset
	// never leaves stale attempt state behind for a later retry.
	if err := s.authStore.ResetResetAttempts(ctx, email); err != nil {
		s.log.Error("clearing reset-attempt counter", slog.String("err", err.Error()))
	}

	// Re-resolve the account fresh at confirm time (same reasoning as
	// commitRegistration: the pending object can be up to 15 minutes stale;
	// don't trust a userID carried in Redis for that long — there isn't one
	// stored here anyway, exactly to avoid that trap).
	userID, err := s.passwordRepo.FindIDByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			// The account vanished between forgot-password and reset. Spend
			// the pending object and report not-found rather than silently
			// no-op — this is an authenticated caller who already proved
			// ownership of the code, not an anonymous prober.
			s.dropPendingReset(ctx, email)
			return ErrNotFound
		}
		return err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcryptCost)
	if err != nil {
		return fmt.Errorf("hashing password: %w", err)
	}
	if err := s.credentialsRepo.Upsert(ctx, userID, string(hash)); err != nil {
		return err
	}

	// "Rules easy to lose": revoke every refresh token, same as
	// commitRegistration's password-overwrite path — a stolen session must
	// not outlive the password it was obtained under.
	if err := s.refreshRepo.RevokeAllForUser(ctx, userID); err != nil {
		return fmt.Errorf("revoking sessions after password reset: %w", err)
	}

	if err := s.authStore.DeletePendingReset(ctx, email); err != nil {
		s.log.Error("deleting spent pending password reset", slog.String("err", err.Error()))
	}

	return nil
}

// ChangePassword changes the password for an authenticated caller (route is
// under AuthMiddleware, userID comes from the verified access token, not
// request input).
//
// If the account has no user_credentials row yet — a Google/Telegram-only
// account setting its FIRST password (ADR-14's second scenario) —
// currentPassword is not checked at all. Otherwise it must match the stored
// hash. Either way, a successful change revokes every refresh token for the
// user (same rule as ResetPassword: a stolen session must not survive a
// password change).
func (s *AuthService) ChangePassword(ctx context.Context, userID int64, currentPassword, newPassword string) error {
	if !dto.ValidPassword(newPassword) {
		return fmt.Errorf("%w: invalid password", ErrInvalidInput)
	}

	creds, err := s.credentialsRepo.Get(ctx, userID)
	switch {
	case err == nil:
		if bcrypt.CompareHashAndPassword([]byte(creds.PasswordHash), []byte(currentPassword)) != nil {
			return ErrUnauthorized
		}
	case errors.Is(err, repo.ErrNotFound):
		// No password ever set for this account — nothing to verify against.
	default:
		return err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcryptCost)
	if err != nil {
		return fmt.Errorf("hashing password: %w", err)
	}
	if err := s.credentialsRepo.Upsert(ctx, userID, string(hash)); err != nil {
		return err
	}

	if err := s.refreshRepo.RevokeAllForUser(ctx, userID); err != nil {
		return fmt.Errorf("revoking sessions after password change: %w", err)
	}

	return nil
}

// sendMailAsync sends an email off the request goroutine (ADR-11): the handler
// answers 202 immediately and never blocks on SMTP. context.WithoutCancel
// keeps the send alive after the request context is cancelled, safego.Go keeps
// a panicking mailer from taking the process down, and the send gets its own
// MAIL_SEND_TIMEOUT bound so a hung relay can't leak goroutines forever.
func (s *AuthService) sendMailAsync(ctx context.Context, to, subject, body string) {
	sendCtx := context.WithoutCancel(ctx)
	log := s.log
	mailer := s.mailer
	metrics := s.mailMetrics
	timeout := s.cfg.MailSendTimeout

	safego.Go(log, func() {
		ctx, cancel := context.WithTimeout(sendCtx, timeout)
		defer cancel()
		if err := mailer.Send(ctx, to, subject, body); err != nil {
			metrics.SendFailure()
			log.Error("sending auth email",
				slog.String("to", to),
				slog.String("err", err.Error()))
		}
	})
}
