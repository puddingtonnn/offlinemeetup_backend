package dto

import (
	"net/mail"
	"regexp"
	"strings"
)

// codeRegexp matches the shape of the emailed confirmation code produced by
// the service layer (six decimal digits — an SMS-style code that is easy to
// retype on a phone).
var codeRegexp = regexp.MustCompile(`^[0-9]{6}$`)

// registrationIDRegexp matches the shape of the registration ID returned by
// /register (AuthService.newRegistrationID: 16 random bytes, hex-encoded).
// It is checked here because the value becomes part of a Redis key — an
// unvalidated one would let a caller inject arbitrary key material.
var registrationIDRegexp = regexp.MustCompile(`^[0-9a-f]{32}$`)

// maxLoginBytes caps the email-or-username login field. 254 is the maximum
// length of an email address per RFC 5321, and usernames are far shorter.
//
// The cap exists because Login feeds the failed-attempt counter's Redis key
// (cache.LoginFailKey) directly: without it, an unauthenticated caller could
// mint arbitrarily large Redis keys, each held for the whole LOGIN_FAIL_WINDOW.
const maxLoginBytes = 254

// validLogin reports whether an email-or-username login field is present and
// within maxLoginBytes. Nothing more is checked: the server decides email vs
// username by the presence of '@' (ADR-2), and a stricter format check here
// would only turn a failed login into a different, more informative error.
func validLogin(login string) bool {
	trimmed := strings.TrimSpace(login)
	return trimmed != "" && len(trimmed) <= maxLoginBytes
}

// Password length policy is measured in BYTES, not runes (ADR-12): bcrypt
// silently truncates its input at 72 bytes, so a 40-character Cyrillic
// password (80 bytes in UTF-8) would have its tail ignored. This is the
// opposite rule from display_name, which is counted in runes.
const (
	minPasswordBytes = 8
	maxPasswordBytes = 72
)

// ValidPassword reports whether a password satisfies the byte-length policy.
// No composition rules (digit/symbol/case) per NIST SP 800-63B.
func ValidPassword(password string) bool {
	n := len([]byte(password))
	return n >= minPasswordBytes && n <= maxPasswordBytes
}

// ValidEmail reports whether s parses as a single addr-spec email address.
func ValidEmail(s string) bool {
	addr, err := mail.ParseAddress(s)
	if err != nil {
		return false
	}
	// ParseAddress accepts `Name <a@b.c>`; we only want the bare address.
	return addr.Address == s
}

// RegisterRequest starts an email/password registration. No user row is
// created until the emailed code is confirmed (ADR-8) — this only produces a
// pending registration in Redis plus an email.
type RegisterRequest struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func (r *RegisterRequest) Validate() map[string]string {
	errs := make(map[string]string)
	if !ValidEmail(strings.TrimSpace(strings.ToLower(r.Email))) {
		errs["email"] = "must be a valid email address"
	}
	if !ValidUsername(r.Username) {
		errs["username"] = "must be 2-32 chars, letters/digits/underscore/dot only"
	}
	if !ValidPassword(r.Password) {
		errs["password"] = "must be between 8 and 72 bytes"
	}
	return errs
}

// RegisterResponse is what /register answers with. RegistrationID identifies
// this one registration attempt and must be echoed back to /verify-email and
// /resend-code; it lets concurrent attempts on the same email coexist instead
// of overwriting each other (see cache.PendingRegKey). It is an addressing
// token, not a secret — the emailed code is what proves ownership.
type RegisterResponse struct {
	RegistrationID string `json:"registration_id"`
}

// VerifyEmailRequest confirms a pending registration. Username is optional and
// only used on the ADR-9 retry path: if the username chosen at register time
// was taken between register and verify, the caller resubmits verify with a
// different one instead of starting over.
type VerifyEmailRequest struct {
	Email          string  `json:"email"`
	RegistrationID string  `json:"registration_id"`
	Code           string  `json:"code"`
	Username       *string `json:"username"`
}

func (r *VerifyEmailRequest) Validate() map[string]string {
	errs := make(map[string]string)
	if !ValidEmail(strings.TrimSpace(strings.ToLower(r.Email))) {
		errs["email"] = "must be a valid email address"
	}
	if !registrationIDRegexp.MatchString(r.RegistrationID) {
		errs["registration_id"] = "must be the 32-char registration_id returned by /register"
	}
	if !codeRegexp.MatchString(r.Code) {
		errs["code"] = "must be a 6-digit code"
	}
	if r.Username != nil && !ValidUsername(*r.Username) {
		errs["username"] = "must be 2-32 chars, letters/digits/underscore/dot only"
	}
	return errs
}

// ResendCodeRequest asks for a fresh confirmation code for a pending
// registration, addressed by the same (email, registration_id) pair as
// VerifyEmailRequest.
type ResendCodeRequest struct {
	Email          string `json:"email"`
	RegistrationID string `json:"registration_id"`
}

func (r *ResendCodeRequest) Validate() map[string]string {
	errs := make(map[string]string)
	if !ValidEmail(strings.TrimSpace(strings.ToLower(r.Email))) {
		errs["email"] = "must be a valid email address"
	}
	if !registrationIDRegexp.MatchString(r.RegistrationID) {
		errs["registration_id"] = "must be the 32-char registration_id returned by /register"
	}
	return errs
}

// LoginRequest authenticates by email OR username + password (ADR-2). Login
// is not format-checked here beyond non-empty — the server decides email vs
// username by the presence of '@' (usernames are validated elsewhere to
// forbid '@', so the two spaces never overlap).
type LoginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

func (r *LoginRequest) Validate() map[string]string {
	errs := make(map[string]string)
	if !validLogin(r.Login) {
		errs["login"] = "must not be empty and at most 254 characters"
	}
	if r.Password == "" {
		errs["password"] = "must not be empty"
	}
	return errs
}

// ForgotPasswordRequest starts a password-reset flow (ADR-14). Login is the
// same email-or-username field as LoginRequest (ADR-2), resolved the same
// way. The response is always 202 regardless of whether login resolves to a
// real account (see AuthService.ForgotPassword) — Validate only rejects an
// empty field, which is a plain client bug, not an account-existence signal.
type ForgotPasswordRequest struct {
	Login string `json:"login"`
}

func (r *ForgotPasswordRequest) Validate() map[string]string {
	errs := make(map[string]string)
	if !validLogin(r.Login) {
		errs["login"] = "must not be empty and at most 254 characters"
	}
	return errs
}

// ResetPasswordRequest completes a forgot-password flow: the emailed code
// plus a new password.
type ResetPasswordRequest struct {
	Email       string `json:"email"`
	Code        string `json:"code"`
	NewPassword string `json:"new_password"`
}

func (r *ResetPasswordRequest) Validate() map[string]string {
	errs := make(map[string]string)
	if !ValidEmail(strings.TrimSpace(strings.ToLower(r.Email))) {
		errs["email"] = "must be a valid email address"
	}
	if !codeRegexp.MatchString(r.Code) {
		errs["code"] = "must be a 6-digit code"
	}
	if !ValidPassword(r.NewPassword) {
		errs["new_password"] = "must be between 8 and 72 bytes"
	}
	return errs
}

// ChangePasswordRequest changes the password of an authenticated caller.
// CurrentPassword is only required by the SERVICE when the account already
// has a password set — a Google/Telegram-only account setting its first
// password legitimately sends it empty, so it is intentionally not checked
// for non-emptiness here.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (r *ChangePasswordRequest) Validate() map[string]string {
	errs := make(map[string]string)
	if !ValidPassword(r.NewPassword) {
		errs["new_password"] = "must be between 8 and 72 bytes"
	}
	return errs
}
