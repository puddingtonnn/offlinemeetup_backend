package http

import (
	"net/http"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/config"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "github.com/puddingtonnn/offlinemeetup_backend/docs"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/handler"
	authMiddleware "github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/middleware"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)

func NewRouter(authHandler *handler.AuthHandler, profileHandler *handler.ProfileHandler, meetupHandler *handler.MeetupHandler, cfg *config.Config) *chi.Mux {
	router := chi.NewRouter()

	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)

	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	router.Group(func(r chi.Router) {
		r.Post("/auth/google", authHandler.GoogleLogin)
		r.Route("/auth/telegram", func(r chi.Router) {
			r.Get("/login", authHandler.ServeTelegramLoginPage)
			r.Get("/callback", authHandler.TelegramCallBack)
		})
	})

	if cfg.Env == "local" || cfg.Env == "dev" {
		router.Post("/auth/dev/login", authHandler.DevLogin)
	}

	router.Route("/v1", func(r chi.Router) {
		r.Use(authMiddleware.AuthMiddleware(cfg))

		r.Get("/auth/me", authHandler.Me)

		r.Route("/profile", func(r chi.Router) {
			r.Get("/", profileHandler.GetMyProfile)
			r.Put("/", profileHandler.UpdateMyProfile)
		})

		r.Route("/meetups", func(r chi.Router) {
			r.Post("/", meetupHandler.CreateMeetup)
			r.Get("/", meetupHandler.List)
			r.Get("/{id}", meetupHandler.GetByID)
			r.Put("/{id}", meetupHandler.Update)
			r.Delete("/{id}", meetupHandler.Delete)
		})
	})

	router.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	return router
}
