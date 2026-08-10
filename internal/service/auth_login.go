package service

import (
	"context"
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/domain"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/repo"
)

// CredentialsRepository is the slice of user-credentials storage the login
// flow needs (ADR-6: the password hash lives in its own table, never on
// UserRepo.GetByID's path). Declared at the consumer; implemented by
// *repo.CredentialsRepo.
type CredentialsRepository interface {
	Get(ctx context.Context, userID int64) (*domain.UserCredentials, error)
}

// dummyPasswordHash is a fixed bcrypt hash (cost bcryptCost, matching
// ADR-12) generated once at package init and compared against whenever the
// real hash isn't available — the login identifier wasn't found, or it was
// found but the account has no credentials row (a Google/Telegram-only
// account that never set a password). Comparing against a real bcrypt hash
// in both cases, rather than short-circuiting, keeps the bcrypt cost paid on
// every login attempt identical regardless of outcome — the "rules easy to
// lose" bullet in GLOBAL-CONSTRAINTS.md: an unknown login must still run
// bcrypt so response timing can't be used to enumerate accounts (ADR-7's
// symmetry, extended from register to login).
var dummyPasswordHash []byte

func init() {
	h, err := bcrypt.GenerateFromPassword([]byte("dummy-password-for-login-timing-parity"), bcryptCost)
	if err != nil {
		// bcrypt only fails on a cost outside [MinCost,MaxCost] or a >72-byte
		// password; bcryptCost is the fixed constant 12 and the string above
		// is well under 72 bytes, so this is unreachable in practice — panic
		// rather than silently start the service with a nil dummy hash (which
		// would make every not-found login a bcrypt no-op, defeating the
		// whole point of this constant).
		panic("service: generating dummy bcrypt hash: " + err.Error())
	}
	dummyPasswordHash = h
}

// isEmailLogin decides email vs username by the presence of '@' (ADR-2) — the
// single place this decision is made; usernames are validated elsewhere
// (dto.ValidUsername) to forbid '@', so the two spaces never overlap.
func isEmailLogin(login string) bool {
	return strings.Contains(login, "@")
}

// Login authenticates by email-or-username + password (ADR-2).
//
// The failed-attempt counter (ADR-13) is incremented FIRST, unconditionally,
// before any lookup or bcrypt work: RedisAuthStore only exposes an atomic
// increment-and-return-count primitive (no separate "peek"), so that single
// increment does double duty as both the threshold check and the recording
// of this attempt. If the returned count is already over the limit, the
// request is rejected immediately without touching Postgres or bcrypt — the
// whole point of the counter (GLOBAL-CONSTRAINTS.md step 1). A successful
// login afterwards resets the counter (ResetLoginFail), so the extra
// increment never accumulates against a legitimate user; a failing login
// simply leaves the already-incremented count in place; no attempt is
// double-counted.
//
// Both "login not found" and "wrong password" return the identical
// ErrUnauthorized sentinel — the same one Refresh already uses for a bad
// token — and both run a real bcrypt comparison (against the real hash when
// one exists, otherwise dummyPasswordHash), so the two failure paths cost
// the same and look the same to the caller (ADR-2's symmetry requirement).
func (s *AuthService) Login(ctx context.Context, login, password string) (*TokenPair, error) {
	count, err := s.authStore.IncrementLoginFail(ctx, login, s.cfg.LoginFailWindow)
	if err != nil {
		return nil, err
	}
	if count > s.cfg.LoginFailLimit {
		return nil, ErrTooManyRequests
	}

	var (
		userID int64
		found  bool
	)
	if isEmailLogin(login) {
		userID, err = s.passwordRepo.FindIDByEmail(ctx, normalizeEmail(login))
	} else {
		userID, err = s.passwordRepo.FindIDByUsername(ctx, strings.TrimSpace(login))
	}
	switch {
	case err == nil:
		found = true
	case errors.Is(err, repo.ErrNotFound):
		found = false
	default:
		return nil, err
	}

	// hasCredentials tracks whether we loaded a REAL hash — as opposed to
	// found being true but the account having no user_credentials row
	// (social-only account). Both "not found" and "found but no
	// credentials" must fall through to the dummy hash and fail identically.
	hash := dummyPasswordHash
	hasCredentials := false
	if found {
		creds, credErr := s.credentialsRepo.Get(ctx, userID)
		switch {
		case credErr == nil:
			hash = []byte(creds.PasswordHash)
			hasCredentials = true
		case errors.Is(credErr, repo.ErrNotFound):
			// No password ever set for this account (e.g. Google-only). Fall
			// through to the dummy hash below.
		default:
			return nil, credErr
		}
	}

	// Always run bcrypt, on every path, real hash or dummy.
	passwordMatches := bcrypt.CompareHashAndPassword(hash, []byte(password)) == nil

	if !hasCredentials || !passwordMatches {
		// Failure path: the counter was already incremented above (step 1);
		// nothing else to write. In particular, no Postgres mutation happens
		// on this path — only the two reads above (FindIDBy*, Credentials.Get).
		return nil, ErrUnauthorized
	}

	if err := s.authStore.ResetLoginFail(ctx, login); err != nil {
		return nil, err
	}
	return s.issueTokenPair(ctx, userID)
}
