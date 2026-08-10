package cache

import (
	"strconv"
	"strings"
)

// Префиксы ключей — единственный источник правды по форматам.
const (
	userChatsPrefix     = "user_chats:"
	tagsAllKey          = "tags:all"
	profilePrefix       = "profile:"
	meetupPrefix        = "meetup:"
	presenceConnsPrefix = "presence:conns:"
	presenceSeenPrefix  = "presence:lastseen:"

	pendingRegPrefix   = "auth:pending_reg:"
	pendingResetPrefix = "auth:pending_reset:"
	loginFailPrefix    = "auth:login_fail:"
	mailCooldownPrefix = "auth:mail_cooldown:"
	mailQuotaPrefix    = "auth:mail_quota:"
)

// UserChatsKey возвращает ключ со списком чатов пользователя.
func UserChatsKey(userID int64) string {
	return userChatsPrefix + strconv.FormatInt(userID, 10)
}

// TagsKey возвращает ключ глобального списка тегов (теги общие для всех).
func TagsKey() string {
	return tagsAllKey
}

// ProfileKey возвращает ключ профиля пользователя (одинаков для всех смотрящих).
func ProfileKey(userID int64) string {
	return profilePrefix + strconv.FormatInt(userID, 10)
}

// MeetupKey возвращает ключ инвариантного снапшота митапа (IsMember накладывается отдельно).
func MeetupKey(meetupID int64) string {
	return meetupPrefix + strconv.FormatInt(meetupID, 10)
}

// PresenceConnsKey — ключ Redis-множества активных WS-соединений пользователя
// (по одному connID на соединение). Пользователь online, пока SCARD > 0.
func PresenceConnsKey(userID int64) string {
	return presenceConnsPrefix + strconv.FormatInt(userID, 10)
}

// PresenceLastSeenKey — ключ с unix-таймстампом последнего ухода в offline.
func PresenceLastSeenKey(userID int64) string {
	return presenceSeenPrefix + strconv.FormatInt(userID, 10)
}

// PendingRegKey — ключ незавершённой регистрации (ADR-8): {password_hash,
// code_hash, attempts, username}, TTL 15 мин, единственный источник правды до
// verify. Email приводится к lower — ADR-3 хранит/ищет email по lower+trim.
func PendingRegKey(email string) string {
	return pendingRegPrefix + strings.ToLower(strings.TrimSpace(email))
}

// PendingResetKey — ключ незавершённого сброса пароля (forgot/reset), та же
// форма, что PendingRegKey, но отдельное пространство ключей.
func PendingResetKey(email string) string {
	return pendingResetPrefix + strings.ToLower(strings.TrimSpace(email))
}

// LoginFailKey — ключ счётчика неудачных попыток входа (ADR-13). login — то,
// что ввёл вызывающий (email или username, ADR-2); приводим к lower здесь,
// чтобы "Denis" и "denis" делили один счётчик.
func LoginFailKey(login string) string {
	return loginFailPrefix + strings.ToLower(strings.TrimSpace(login))
}

// MailCooldownKey — ключ анти-даблклик кулдауна на повторную отправку письма.
func MailCooldownKey(email string) string {
	return mailCooldownPrefix + strings.ToLower(strings.TrimSpace(email))
}

// MailQuotaKey — ключ часовой квоты на отправку писем для одного email.
func MailQuotaKey(email string) string {
	return mailQuotaPrefix + strings.ToLower(strings.TrimSpace(email))
}
