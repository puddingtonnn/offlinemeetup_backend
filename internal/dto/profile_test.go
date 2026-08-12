package dto

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidUsername(t *testing.T) {
	tests := []struct {
		name     string
		username string
		want     bool
	}{
		{name: "simple lowercase", username: "bob", want: true},
		{name: "with underscore and digit", username: "bob_2", want: true},
		{name: "with dot", username: "bob.smith", want: true},
		{name: "minimum length (2 chars)", username: "bo", want: true},
		{name: "maximum length (32 chars)", username: strings.Repeat("a", 32), want: true},
		{name: "too short (1 char)", username: "b", want: false},
		{name: "too long (33 chars)", username: strings.Repeat("a", 33), want: false},
		{name: "empty string", username: "", want: false},
		{
			// Load-bearing for isEmailLogin's email-vs-username
			// disambiguation (ADR-2): '@' must never be a valid username
			// character, or the login endpoint couldn't tell the two
			// apart by presence of '@'.
			name:     "contains @ must be rejected",
			username: "bob@example.com",
			want:     false,
		},
		{name: "contains space", username: "bob smith", want: false},
		{name: "contains disallowed symbol", username: "bob!smith", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ValidUsername(tt.username))
		})
	}
}
