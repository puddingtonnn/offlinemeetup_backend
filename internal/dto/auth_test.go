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
