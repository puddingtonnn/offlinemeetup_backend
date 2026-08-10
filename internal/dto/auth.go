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

// VerifyEmailRequest confirms a pending registration. Username is optional and
// only used on the ADR-9 retry path: if the username chosen at register time
// was taken between register and verify, the caller resubmits verify with a
// different one instead of starting over.
type VerifyEmailRequest struct {
	Email    string  `json:"email"`
	Code     string  `json:"code"`
	Username *string `json:"username"`
}

func (r *VerifyEmailRequest) Validate() map[string]string {
	errs := make(map[string]string)
	if !ValidEmail(strings.TrimSpace(strings.ToLower(r.Email))) {
		errs["email"] = "must be a valid email address"
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
// registration.
type ResendCodeRequest struct {
	Email string `json:"email"`
}

func (r *ResendCodeRequest) Validate() map[string]string {
	errs := make(map[string]string)
	if !ValidEmail(strings.TrimSpace(strings.ToLower(r.Email))) {
		errs["email"] = "must be a valid email address"
	}
	return errs
}
