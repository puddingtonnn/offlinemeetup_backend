package middleware

import (
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// rateLimitScript атомарно инкрементирует счётчик окна и на первом хите ставит
// ему TTL, возвращая текущее значение. Классический fixed-window лимитер; INCR и
// PEXPIRE в одном скрипте исключают гонку «ключ без TTL».
var rateLimitScript = redis.NewScript(`
local c = redis.call("INCR", KEYS[1])
if c == 1 then
	redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
return c
`)

// RateLimiter ограничивает число запросов с одного IP до limit за окно window,
// считая в Redis — счётчик общий для всех инстансов. scope разносит счётчики
// разных групп эндпоинтов по разным ключам. Лимитер best-effort: ошибка Redis
// не блокирует запрос (fail-open), чтобы сбой кеша не положил аутентификацию.
func RateLimiter(rdb *redis.Client, log *slog.Logger, scope string, limit int64, window time.Duration, trustProxy bool) func(http.Handler) http.Handler {
	windowMs := window.Milliseconds()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := "ratelimit:" + scope + ":" + clientIP(r, trustProxy)
			count, err := rateLimitScript.Run(r.Context(), rdb, []string{key}, windowMs).Int64()
			if err != nil {
				log.Warn("rate limiter: redis error, allowing request", slog.Any("err", err))
				next.ServeHTTP(w, r)
				return
			}
			if count > limit {
				w.Header().Set("Retry-After", strconv.Itoa(int(window.Seconds())))
				http.Error(w, "too many requests", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientIP извлекает IP клиента для ключа rate-limit. Заголовкам прокси
// (X-Real-IP / X-Forwarded-For) доверяем ТОЛЬКО когда trustProxy=true — их может
// подделать любой клиент, а RemoteAddr подделать нельзя. За доверенным прокси,
// который сам перезаписывает эти заголовки, они надёжны; при прямом подключении
// (trustProxy=false) всегда используем RemoteAddr.
func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xrip := strings.TrimSpace(r.Header.Get("X-Real-IP")); xrip != "" {
			return xrip
		}
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			first, _, _ := strings.Cut(xff, ",")
			return strings.TrimSpace(first)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
