package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDisplayNameOf(t *testing.T) {
	nonEmpty := "Bob Smith"
	empty := ""

	tests := []struct {
		name        string
		username    string
		displayName *string
		want        string
	}{
		{
			name:        "display_name set returns display_name",
			username:    "bob",
			displayName: &nonEmpty,
			want:        "Bob Smith",
		},
		{
			name:        "nil display_name falls back to username",
			username:    "bob",
			displayName: nil,
			want:        "bob",
		},
		{
			name:        "empty-string display_name falls back to username",
			username:    "bob",
			displayName: &empty,
			want:        "bob",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, DisplayNameOf(tt.username, tt.displayName))
		})
	}
}
