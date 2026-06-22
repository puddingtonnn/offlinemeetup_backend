package websocket

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckOrigin(t *testing.T) {
	tests := []struct {
		name    string
		allowed []string
		origin  string
		want    bool
	}{
		{"empty allowlist allows any", nil, "https://evil.com", true},
		{"empty origin allowed (native client)", []string{"https://app.example.com"}, "", true},
		{"origin in allowlist", []string{"https://app.example.com"}, "https://app.example.com", true},
		{"origin not in allowlist", []string{"https://app.example.com"}, "https://evil.com", false},
		{"allowlist with spaces", []string{" https://app.example.com "}, "https://app.example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			up := newUpgrader(tt.allowed)
			req := httptest.NewRequest(http.MethodGet, "/ws", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			assert.Equal(t, tt.want, up.CheckOrigin(req))
		})
	}
}
