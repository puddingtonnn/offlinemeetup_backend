package middleware

import "net/http"

// SecurityHeaders ставит консервативные защитные заголовки на каждый ответ.
// API обслуживает нативные мобильные клиенты плюс пару HTML-поверхностей
// (страница Telegram-логина, Swagger в dev), поэтому эти заголовки ничего не
// стоят и закрывают браузерные пути: nosniff запрещает MIME-sniffing, DENY —
// фрейминг (кликджекинг), no-referrer не утекает URL, no-store не даёт
// прокси/браузеру кешировать ответы, специфичные для пользователя.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
