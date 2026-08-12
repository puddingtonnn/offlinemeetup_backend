package mail

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestLogMailer_Send_LogsRecipientSubjectAndBody(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	m := NewLogMailer(log)

	err := m.Send(context.Background(), "user@example.com", "Confirm your Meetuper registration", "code: 123456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	for _, want := range []string{"user@example.com", "Confirm your Meetuper registration", "123456"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected log output to contain %q, got: %q", want, out)
		}
	}
}
