package middleware

import (
	"net/http"
	"strings"
)

// BodyLimit ограничивает размер тела запроса для защиты от DoS большими payload'ами.
// multipart-запросы (загрузка файлов) пропускаются — их лимит задаётся в своём хендлере.
func BodyLimit(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil && !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}
