package mail

import (
	"fmt"
	"time"
)

// Templates for the three emails the email/password auth flows send (see
// ADR-1/ADR-7/ADR-14 in the plan). All three carry a one-time numeric code;
// the caller decides the code's format/length — this file only renders copy
// around it. Each function returns (subject, body), ready to pass to
// Mailer.Send.
//
// Copy is Russian, matching the app's audience (the rest of the product —
// tag names, API docs — is Russian too).
//
// ttl is the code's lifetime, passed in rather than hardcoded as "15 минут":
// it is configurable (EMAIL_CODE_TTL) and a letter that promises a different
// number from the one the server enforces is a support ticket waiting to
// happen.

// expiryPhrase renders a code lifetime as Russian minutes with the right
// numeral agreement ("1 минуту" / "3 минуты" / "15 минут"). A TTL under a
// minute rounds up to 1 — the copy only ever needs whole minutes, and "0
// минут" would be nonsense.
func expiryPhrase(ttl time.Duration) string {
	minutes := max(int(ttl.Minutes()), 1)

	// Russian numeral agreement: 11–14 are the exception that makes a plain
	// last-digit switch wrong (11 минут, not 11 минута).
	word := "минут"
	switch mod100 := minutes % 100; {
	case mod100 >= 11 && mod100 <= 14:
	default:
		switch minutes % 10 {
		case 1:
			word = "минуту"
		case 2, 3, 4:
			word = "минуты"
		}
	}
	return fmt.Sprintf("%d %s", minutes, word)
}

// RegistrationVerification renders the email sent when a brand-new
// registration is started (ADR-1: confirms ownership of the email before the
// account is created). username is the username the caller chose at
// register time.
func RegistrationVerification(username, code string, ttl time.Duration) (subject, body string) {
	subject = "Подтвердите регистрацию в Meetuper"
	body = fmt.Sprintf(
		"Привет, %s!\n\n"+
			"Введите код ниже, чтобы подтвердить email и завершить создание аккаунта Meetuper:\n\n"+
			"    %s\n\n"+
			"Код действует %s. Если вы этого не запрашивали — просто проигнорируйте письмо.\n",
		username, code, expiryPhrase(ttl),
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
func ExistingAccountVerification(code string, ttl time.Duration) (subject, body string) {
	subject = "Кто-то пытался зарегистрироваться с вашим email в Meetuper"
	body = fmt.Sprintf(
		"Здравствуйте!\n\n"+
			"Кто-то только что попытался создать новый аккаунт Meetuper с этим адресом, "+
			"но аккаунт с ним уже существует.\n\n"+
			"Если это вы и хотите добавить вход по паролю к существующему аккаунту, "+
			"подтвердите это кодом:\n\n"+
			"    %s\n\n"+
			"Код действует %s. Если это были не вы — делать ничего не нужно: аккаунт в порядке, "+
			"пароль не установлен.\n",
		code, expiryPhrase(ttl),
	)
	return subject, body
}

// PasswordReset renders the email sent for a forgot-password request
// (ADR-14). username personalizes the greeting and may be empty — the reset
// flow deliberately doesn't load a profile just to say hello (see
// AuthService.forgotPasswordAsync), so an empty name falls back to a neutral
// greeting instead of "Привет, !".
func PasswordReset(username, code string, ttl time.Duration) (subject, body string) {
	greeting := "Здравствуйте!"
	if username != "" {
		greeting = fmt.Sprintf("Привет, %s!", username)
	}
	subject = "Восстановление пароля Meetuper"
	body = fmt.Sprintf(
		"%s\n\n"+
			"Введите код ниже, чтобы задать новый пароль в Meetuper:\n\n"+
			"    %s\n\n"+
			"Код действует %s. Если вы не запрашивали смену пароля — проигнорируйте письмо, "+
			"пароль останется прежним.\n",
		greeting, code, expiryPhrase(ttl),
	)
	return subject, body
}
