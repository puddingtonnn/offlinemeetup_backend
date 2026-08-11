package mail

import (
	"context"
	"fmt"
	"strconv"

	gomail "github.com/wneessen/go-mail"
)

// smtpMailer sends real email through an SMTP relay via
// github.com/wneessen/go-mail. Used everywhere APP_ENV is not local/dev (see
// internal/app.New); config.Load already fails fast outside local/dev if any
// of MAIL_SMTP_HOST/PORT/USER/PASSWORD/FROM is empty, so smtpMailer can
// assume its fields are non-empty.
type smtpMailer struct {
	client *gomail.Client
	from   string
}

// NewSMTPMailer builds a Mailer backed by an SMTP relay. port is the numeric
// SMTP port (config.MailSMTPPort, e.g. "587"); it is parsed once here so a
// bad env value fails at startup, not on the first send.
func NewSMTPMailer(host, port, user, password, from string) (Mailer, error) {
	portNum, err := strconv.Atoi(port)
	if err != nil {
		return nil, fmt.Errorf("mail: invalid MAIL_SMTP_PORT %q: %w", port, err)
	}

	client, err := gomail.NewClient(host,
		gomail.WithPort(portNum),
		gomail.WithSMTPAuth(gomail.SMTPAuthPlain),
		gomail.WithUsername(user),
		gomail.WithPassword(password),
		gomail.WithTLSPolicy(gomail.TLSMandatory),
	)
	if err != nil {
		return nil, fmt.Errorf("mail: building smtp client: %w", err)
	}

	return &smtpMailer{client: client, from: from}, nil
}

// Send implements Mailer.
func (m *smtpMailer) Send(ctx context.Context, to, subject, body string) error {
	msg := gomail.NewMsg()
	if err := msg.From(m.from); err != nil {
		return fmt.Errorf("mail: setting from address: %w", err)
	}
	if err := msg.To(to); err != nil {
		return fmt.Errorf("mail: setting to address: %w", err)
	}
	msg.Subject(subject)
	msg.SetBodyString(gomail.TypeTextPlain, body)

	if err := m.client.DialAndSendWithContext(ctx, msg); err != nil {
		return fmt.Errorf("mail: sending via smtp: %w", err)
	}
	return nil
}
