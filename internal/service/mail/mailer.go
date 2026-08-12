// Package mail provides the outbound-email seam used by the email/password
// auth flows (registration verification, password-attach-to-existing-account,
// password reset). It defines the Mailer interface, a logMailer stub for
// local/dev, an smtpMailer relay implementation (github.com/wneessen/go-mail)
// for everywhere else, and the plain-text templates used by those flows.
package mail

import "context"

// Mailer sends a single plain-text email. Implementations must be safe for
// concurrent use, since sends happen from background goroutines (see
// safego.Go + context.WithoutCancel at the call site — the HTTP response
// returns before the send completes, so ctx passed here is typically already
// detached from the originating request's cancellation).
//
// to is the recipient address. subject and body are plain text (no HTML
// templating in this codebase yet) — see templates.go for the copy used by
// each auth flow.
type Mailer interface {
	Send(ctx context.Context, to, subject, body string) error
}
