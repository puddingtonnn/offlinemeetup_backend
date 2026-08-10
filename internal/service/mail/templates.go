package mail

import "fmt"

// Templates for the three emails the email/password auth flows send (see
// ADR-1/ADR-7/ADR-14 in the plan). All three carry a one-time numeric/
// alphanumeric code; the caller decides the code's format/length and TTL —
// this file only renders copy around it. Each function returns
// (subject, body), ready to pass to Mailer.Send.

// RegistrationVerification renders the email sent when a brand-new
// registration is started (ADR-1: confirms ownership of the email before the
// account is created). username is the username the caller chose at
// register time.
func RegistrationVerification(username, code string) (subject, body string) {
	subject = "Confirm your Meetuper registration"
	body = fmt.Sprintf(
		"Hi %s,\n\n"+
			"Use the code below to confirm your email and finish creating your Meetuper account:\n\n"+
			"    %s\n\n"+
			"This code expires in 15 minutes. If you didn't request this, you can ignore this email.\n",
		username, code,
	)
	return subject, body
}

// ExistingAccountVerification renders the email sent when someone tries to
// register with an email that already has a Meetuper account (e.g. created
// via Google/Telegram login). Per ADR-7 the outward API response is
// identical to a fresh registration, but the email copy makes it clear a
// password login is being *added* to an existing account, not a new one
// being created — and that this wasn't necessarily requested by the account
// owner.
func ExistingAccountVerification(code string) (subject, body string) {
	subject = "Someone tried to register with your Meetuper email"
	body = fmt.Sprintf(
		"Hi,\n\n"+
			"Someone just tried to register a new Meetuper account using this email address, "+
			"but an account already exists for it.\n\n"+
			"If this was you and you'd like to add a password login to your existing account, "+
			"use the code below to confirm:\n\n"+
			"    %s\n\n"+
			"This code expires in 15 minutes. If this wasn't you, no action is needed — your account is safe "+
			"and no password has been set.\n",
		code,
	)
	return subject, body
}

// PasswordReset renders the email sent for a forgot-password request
// (ADR-14). username is the account's username, used only to personalize
// the copy.
func PasswordReset(username, code string) (subject, body string) {
	subject = "Reset your Meetuper password"
	body = fmt.Sprintf(
		"Hi %s,\n\n"+
			"Use the code below to reset your Meetuper password:\n\n"+
			"    %s\n\n"+
			"This code expires in 15 minutes. If you didn't request a password reset, you can ignore this "+
			"email — your password won't be changed.\n",
		username, code,
	)
	return subject, body
}
