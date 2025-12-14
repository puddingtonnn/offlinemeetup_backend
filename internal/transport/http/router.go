package http

import (
	"github.com/puddingtonnn/offlinemeetup_backend/internal/config"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "github.com/puddingtonnn/offlinemeetup_backend/docs"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/handler"
	authMiddleware "github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/middleware"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)

func NewRouter(authHandler *handler.AuthHandler, profileHandler *handler.ProfileHandler, cfg *config.Config) *chi.Mux {
	router := chi.NewRouter()

	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)

	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	router.Group(func(r chi.Router) {
		r.Post("/auth/google", authHandler.GoogleLogin)
		r.Post("/auth/telegram", authHandler.TelegramLogin)
	})

	router.Group(func(r chi.Router) {
		r.Use(authMiddleware.AuthMiddleware(cfg))
		r.Get("/auth/me", authHandler.Me)

		r.Get("/api/profile", profileHandler.GetMyProfile)
		r.Put("/api/profile", profileHandler.UpdateMyProfile)
	})

	router.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	return router
}
