package mail

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/joho/godotenv"
)

// TestSMTPSmoke_SendsRealEmail actually talks to the configured SMTP relay and
// delivers one message. It is SKIPPED unless SMTP_SMOKE_TO names a mailbox to
// send to, so `go test ./...` never dials the network:
//
//	SMTP_SMOKE_TO=you@example.com go test ./internal/service/mail/ -run SMTPSmoke -v
//
// It exercises the production path exactly — same NewSMTPMailer constructor,
// same go-mail client, same TLS policy — so a pass means the credentials, the
// port and the relay's auth all work, not just that the config parses.
//
// Relay credentials come from the repo-root .env (the same file the app
// reads); real environment variables win over it, so CI or a one-off shell
// export can point this at a different relay without editing the file.
func TestSMTPSmoke_SendsRealEmail(t *testing.T) {
	to := os.Getenv("SMTP_SMOKE_TO")
	if to == "" {
		t.Skip("set SMTP_SMOKE_TO=<mailbox> to send a real email through the configured relay")
	}

	// Best-effort: if the file is missing, the env vars may already be set.
	_ = godotenv.Load(filepath.Join("..", "..", "..", ".env"))

	host := os.Getenv("MAIL_SMTP_HOST")
	if host == "" {
		t.Fatal("MAIL_SMTP_HOST is empty — nothing to smoke-test against")
	}
	port := os.Getenv("MAIL_SMTP_PORT")
	if port == "" {
		port = "587"
	}
	from := os.Getenv("MAIL_FROM")

	mailer, err := NewSMTPMailer(host, port, os.Getenv("MAIL_SMTP_USER"), os.Getenv("MAIL_SMTP_PASSWORD"), from)
	if err != nil {
		t.Fatalf("building smtp mailer: %v", err)
	}

	// A real template, so the smoke test also shows what the copy looks like
	// in an actual inbox (encoding, line breaks, the code block).
	subject, body := RegistrationVerification("smoke-test", "424242", 15*time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := mailer.Send(ctx, to, subject, body); err != nil {
		// A timeout means the relay never sent a single byte — the TCP
		// handshake can still "succeed" against a VPN/proxy TUN device that
		// then fails to carry the connection. Worth calling out, because it
		// looks nothing like a credentials problem and says nothing about
		// whether the credentials are right: outbound 25/465/587 is commonly
		// blocked by ISPs, and split-tunnel clients routinely forward 443
		// while dropping SMTP. Check `route get <relay ip>` for a utun
		// interface before touching the password.
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			t.Fatalf("no response from %s:%s within the timeout — the relay was never reached, "+
				"so this says nothing about the credentials. Check for a VPN/proxy capturing the "+
				"route (`route get`) or an ISP block on outbound SMTP. Underlying error: %v",
				host, port, err)
		}
		t.Fatalf("sending via %s:%s as %q: %v", host, port, from, err)
	}
	t.Logf("sent %q to %s via %s:%s — check the inbox (and the spam folder)", subject, to, host, port)
}
