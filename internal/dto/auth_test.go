package dto

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidPassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		want     bool
	}{
		{
			name:     "too short (7 bytes)",
			password: "short12",
			want:     false,
		},
		{
			name:     "minimum valid length (8 bytes)",
			password: "12345678",
			want:     true,
		},
		{
			name:     "comfortably valid (20 bytes)",
			password: "a-perfectly-fine-pw!",
			want:     true,
		},
		{
			name:     "maximum valid length (72 bytes)",
			password: strings.Repeat("a", 72),
			want:     true,
		},
		{
			name:     "too long (73 bytes)",
			password: strings.Repeat("a", 73),
			want:     false,
		},
		{
			// ADR-12: bcrypt truncates at 72 BYTES, not runes/characters.
			// Cyrillic is 2 bytes/letter in UTF-8, so 40 characters is 80
			// bytes — over the limit even though it's well under 72
			// characters. This is the specific trap the byte-length rule
			// exists to catch.
			name:     "40 Cyrillic characters (80 bytes) must be rejected",
			password: strings.Repeat("п", 40),
			want:     false,
		},
		{
			// 20 Cyrillic characters = 40 bytes, comfortably within the
			// 8-72 byte window.
			name:     "20 Cyrillic characters (40 bytes) is accepted",
			password: strings.Repeat("п", 20),
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ValidPassword(tt.password))
		})
	}
}

// The login field is capped because it becomes part of a Redis key (the
// failed-attempt counter, cache.LoginFailKey) — an uncapped one lets an
// unauthenticated caller mint arbitrarily large keys, each held for the whole
// LOGIN_FAIL_WINDOW.
func TestLoginRequest_Validate_CapsLoginLength(t *testing.T) {
	tests := []struct {
		name    string
		login   string
		wantErr bool
	}{
		{name: "ordinary email", login: "bob@example.com"},
		{name: "ordinary username", login: "bob"},
		{name: "empty", login: "", wantErr: true},
		{name: "whitespace only", login: "   ", wantErr: true},
		{name: "at the cap (254)", login: strings.Repeat("a", 254)},
		{name: "over the cap (255)", login: strings.Repeat("a", 255), wantErr: true},
		{name: "cap applies after trimming", login: "  " + strings.Repeat("a", 254) + "  "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := (&LoginRequest{Login: tt.login, Password: "a good password"}).Validate()
			_, gotErr := errs["login"]
			assert.Equal(t, tt.wantErr, gotErr)
		})
	}
}

// ForgotPasswordRequest takes the same email-or-username field, so it needs
// the same cap.
func TestForgotPasswordRequest_Validate_CapsLoginLength(t *testing.T) {
	assert.NotContains(t, (&ForgotPasswordRequest{Login: "bob@example.com"}).Validate(), "login")
	assert.Contains(t, (&ForgotPasswordRequest{Login: ""}).Validate(), "login")
	assert.Contains(t, (&ForgotPasswordRequest{Login: strings.Repeat("a", 255)}).Validate(), "login")
}

// registration_id also lands in a Redis key, and unlike the login it has an
// exact known shape (32 lowercase hex chars from AuthService.Register), so it
// is checked strictly.
func TestVerifyEmailRequest_Validate_RegistrationID(t *testing.T) {
	valid := "0123456789abcdef0123456789abcdef"

	tests := []struct {
		name    string
		regID   string
		wantErr bool
	}{
		{name: "well-formed", regID: valid},
		{name: "empty", regID: "", wantErr: true},
		{name: "too short", regID: "0123456789abcdef", wantErr: true},
		{name: "too long", regID: valid + "00", wantErr: true},
		{name: "uppercase hex", regID: strings.ToUpper(valid), wantErr: true},
		{name: "non-hex characters", regID: strings.Repeat("z", 32), wantErr: true},
		{name: "redis key separator injected", regID: "0123456789abcdef0123456789abcde:", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &VerifyEmailRequest{Email: "bob@example.com", RegistrationID: tt.regID, Code: "123456"}
			_, gotErr := req.Validate()["registration_id"]
			assert.Equal(t, tt.wantErr, gotErr)

			resend := &ResendCodeRequest{Email: "bob@example.com", RegistrationID: tt.regID}
			_, gotErr = resend.Validate()["registration_id"]
			assert.Equal(t, tt.wantErr, gotErr, "resend must enforce the same shape")
		})
	}
}

func TestValidEmail(t *testing.T) {
	tests := []struct {
		name  string
		email string
		want  bool
	}{
		{name: "valid simple address", email: "bob@example.com", want: true},
		{name: "valid with subdomain", email: "bob.smith@mail.example.com", want: true},
		{name: "empty string", email: "", want: false},
		{name: "missing @", email: "bob.example.com", want: false},
		{name: "missing domain", email: "bob@", want: false},
		{name: "missing local part", email: "@example.com", want: false},
		{name: "name-addr form is rejected (bare address only)", email: "Bob <bob@example.com>", want: false},
		{name: "trailing space", email: "bob@example.com ", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ValidEmail(tt.email))
		})
	}
}
