package handler

import "net/http"

func (h *AuthHandler) ServeTelegramLoginPage(w http.ResponseWriter, r *http.Request) {
	html := `
    <!DOCTYPE html>
    <html>
    <head><title>Login</title></head>
    <body>
        <div style="display:flex; justify-content:center; align-items:center; height:100vh;">
            <script async src="https://telegram.org/js/telegram-widget.js?22" 
                data-telegram-login="ИМЯ_ТВОЕГО_БОТА" 
                data-size="large" 
                data-auth-url="https://ТВОЙ_ДОМЕН/auth/telegram/callback" 
                data-request-access="write"></script>
        </div>
    </body>
    </html>`

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}
