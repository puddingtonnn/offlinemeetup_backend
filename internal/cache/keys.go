package cache

import "strconv"

// Префиксы ключей — единственный источник правды по форматам.
const (
	userChatsPrefix     = "user_chats:"
	tagsAllKey          = "tags:all"
	profilePrefix       = "profile:"
	meetupPrefix        = "meetup:"
	presenceConnsPrefix = "presence:conns:"
	presenceSeenPrefix  = "presence:lastseen:"
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
