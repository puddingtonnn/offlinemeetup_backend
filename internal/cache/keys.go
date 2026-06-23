package cache

import "strconv"

// userChatsPrefix — неймспейс для кешированного списка чатов пользователя.
const userChatsPrefix = "user_chats:"

// UserChatsKey возвращает ключ со списком чатов пользователя — единственный источник правды по формату.
func UserChatsKey(userID int64) string {
	return userChatsPrefix + strconv.FormatInt(userID, 10)
}
