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
// registration object (ADR-8) plus the mail cooldown/quota throttles.
// Implemented by *cache.RedisAuthStore.
type AuthStore interface {
	SavePendingReg(ctx context.Context, email string, data cache.PendingReg, ttl time.Duration) error
	GetPendingReg(ctx context.Context, email string) (cache.PendingReg, bool, error)
	DeletePendingReg(ctx context.Context, email string) error
	IncrementPendingRegAttempts(ctx context.Context, email string) (int, error)
	CheckAndSetMailCooldown(ctx context.Context, email string, cooldown time.Duration) (bool, error)
	IncrementMailQuota(ctx context.Context, email string, window time.Duration) (int, error)
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

// Register starts an email/password registration (ADR-1, ADR-8). Nothing is
// written to Postgres: the password hash, the code hash and the chosen
// username live in Redis for EMAIL_CODE_TTL, and a repeated register on the
// same email simply overwrites that object. The response is identical whether
// or not the email already has an account (ADR-7) — only the email template
// differs — so the endpoint is not an account-existence oracle.
func (s *AuthService) Register(ctx context.Context, email, username, password string) error {
	email = normalizeEmail(email)
	username = strings.TrimSpace(username)

	if !dto.ValidEmail(email) {
		return fmt.Errorf("%w: invalid email", ErrInvalidInput)
	}
	if !dto.ValidUsername(username) {
		return fmt.Errorf("%w: invalid username", ErrInvalidInput)
	}
	if !dto.ValidPassword(password) {
		return fmt.Errorf("%w: invalid password", ErrInvalidInput)
	}

	// Soft (non-authoritative) username check: a fast 400 so we don't email a
	// registration that is already doomed. The unique index checked inside
	// VerifyEmail's transaction is the real arbiter (ADR-9).
	switch _, err := s.passwordRepo.FindIDByUsername(ctx, username); {
	case err == nil:
		return fmt.Errorf("%w: username already taken", ErrInvalidInput)
	case errors.Is(err, repo.ErrNotFound):
		// free (for now)
	default:
		return err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return fmt.Errorf("hashing password: %w", err)
	}

	code, err := generateCode()
	if err != nil {
		return err
	}

	// ADR-7: whether the email already has an account changes only which
	// template we send, never the status code or body shape.
	accountExists := false
	switch _, err := s.passwordRepo.FindIDByEmail(ctx, email); {
	case err == nil:
		accountExists = true
	case errors.Is(err, repo.ErrNotFound):
	default:
		return err
	}

	pending := cache.PendingReg{
		PasswordHash: string(hash),
		CodeHash:     hashCode(code),
		Attempts:     0,
		Username:     username,
	}
	if err := s.authStore.SavePendingReg(ctx, email, pending, s.cfg.EmailCodeTTL); err != nil {
		return err
	}

	// The hourly quota is enforced here too, not just on resend: otherwise
	// repeating `register` is a free way to flood a stranger's inbox. Over
	// quota we skip the send but still return success — same response shape,
	// no new status code (the plan's API table has no 429 on register).
	if !s.reserveMailQuota(ctx, email) {
		s.log.Warn("registration email suppressed by hourly quota", slog.String("email", email))
		return nil
	}

	var subject, body string
	if accountExists {
		subject, body = mail.ExistingAccountVerification(code)
	} else {
		subject, body = mail.RegistrationVerification(username, code)
	}
	s.sendMailAsync(ctx, email, subject, body)

	return nil
}

// VerifyEmail confirms a pending registration and logs the caller in.
//
// chosenUsername is the ADR-9 retry path: if the username picked at register
// time was taken in the meantime, verify returns 409 WITHOUT destroying the
// pending object, and the caller resubmits with a different username.
func (s *AuthService) VerifyEmail(ctx context.Context, email, code string, chosenUsername *string) (*TokenPair, error) {
	email = normalizeEmail(email)

	pending, found, err := s.authStore.GetPendingReg(ctx, email)
	if err != nil {
		return nil, err
	}
	if !found {
		// Never started, or the 15-minute window elapsed.
		return nil, fmt.Errorf("%w: no pending registration", ErrInvalidInput)
	}

	if pending.Attempts >= s.cfg.EmailCodeMaxAttempts {
		s.dropPending(ctx, email)
		return nil, fmt.Errorf("%w: too many invalid codes", ErrTooManyRequests)
	}

	if !codeMatches(code, pending.CodeHash) {
		attempts, err := s.authStore.IncrementPendingRegAttempts(ctx, email)
		if err != nil {
			if errors.Is(err, cache.ErrPendingNotFound) {
				return nil, fmt.Errorf("%w: no pending registration", ErrInvalidInput)
			}
			return nil, err
		}
		if attempts >= s.cfg.EmailCodeMaxAttempts {
			// Burned through the allowance: invalidate the pending object so the
			// remaining guesses can't be spent, and make the caller start over.
			s.dropPending(ctx, email)
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
	if err := s.authStore.DeletePendingReg(ctx, email); err != nil {
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

// ResendCode issues a fresh confirmation code for a pending registration.
//
// It always reports success, including when there is no pending registration
// for the email: the plan's API table lists only 202/429 for this endpoint, and
// a 400-on-unknown-email would turn resend into the account-enumeration oracle
// that ADR-7 spends the whole register flow avoiding. The throttles are applied
// BEFORE the pending lookup so a prober can't distinguish the two cases by
// which limits get consumed either.
func (s *AuthService) ResendCode(ctx context.Context, email string) error {
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

	pending, found, err := s.authStore.GetPendingReg(ctx, email)
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
	pending.CodeHash = hashCode(code)
	pending.Attempts = 0
	if err := s.authStore.SavePendingReg(ctx, email, pending, s.cfg.EmailCodeTTL); err != nil {
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
		subject, body = mail.ExistingAccountVerification(code)
	} else {
		subject, body = mail.RegistrationVerification(pending.Username, code)
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
func (s *AuthService) dropPending(ctx context.Context, email string) {
	if err := s.authStore.DeletePendingReg(ctx, email); err != nil {
		s.log.Error("invalidating pending registration", slog.String("err", err.Error()))
	}
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
	timeout := s.cfg.MailSendTimeout

	safego.Go(log, func() {
		ctx, cancel := context.WithTimeout(sendCtx, timeout)
		defer cancel()
		if err := mailer.Send(ctx, to, subject, body); err != nil {
			log.Error("sending auth email",
				slog.String("to", to),
				slog.String("err", err.Error()))
		}
	})
}
