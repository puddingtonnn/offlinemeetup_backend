package mail

import (
	"context"
	"log/slog"
)

// logMailer is the Mailer used for local/dev: instead of sending a real
// email it logs the recipient, subject, and body. The body of the
// registration/reset templates contains the plaintext verification code, so
// this is how a human tester retrieves it locally (`make up` → register →
// read the code from `make logs`). Each field is a structured slog attr
// (not buried in a formatted message) so it's easy to grep/spot.
type logMailer struct {
	log *slog.Logger
}

// NewLogMailer builds a Mailer that logs instead of sending. Use it for
// local/dev environments.
func NewLogMailer(log *slog.Logger) Mailer {
	return &logMailer{log: log}
}

// Send implements Mailer.
func (m *logMailer) Send(_ context.Context, to, subject, body string) error {
	m.log.Info("mail: send (dev mode, not actually delivered)",
		slog.String("to", to),
		slog.String("subject", subject),
		slog.String("body", body),
	)
	return nil
}
