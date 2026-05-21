package http

import (
	"net/http"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/config"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/websocket"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "github.com/puddingtonnn/offlinemeetup_backend/docs"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/handler"
	authMiddleware "github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/middleware"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)

func NewRouter(authHandler *handler.AuthHandler,
	profileHandler *handler.ProfileHandler,
	meetupHandler *handler.MeetupHandler,
	tagHandler *handler.TagHandler,
	geoHandler *handler.GeoHandler,
	chatHandler *handler.ChatHandler,
	wsHandler *websocket.WSHandler,
	fileHandler *handler.FileHandler,
	cfg *config.Config) *chi.Mux {
	router := chi.NewRouter()

	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)

	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	if cfg.Env == "local" || cfg.Env == "dev" {
		router.Post("/auth/dev/login", authHandler.DevLogin)
	}

	router.Route("/v1", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Post("/auth/google", authHandler.GoogleLogin)
			r.Route("/auth/telegram", func(r chi.Router) {
				r.Get("/login", authHandler.ServeTelegramLoginPage)
				r.Get("/callback", authHandler.TelegramCallBack)
			})
			r.Get("/tags", tagHandler.List)
		})

		r.Route("/meetups", func(r chi.Router) {
			r.Group(func(r chi.Router) {
				r.Use(authMiddleware.UserIdentityMiddleware(cfg))

				r.Get("/", meetupHandler.List)
				r.Get("/{id}", meetupHandler.GetByID)
			})

			r.Group(func(r chi.Router) {
				r.Use(authMiddleware.AuthMiddleware(cfg))

				r.Post("/", meetupHandler.CreateMeetup)
				r.Put("/{id}", meetupHandler.Update)
				r.Delete("/{id}", meetupHandler.Delete)
				r.Post("/{id}/join", meetupHandler.Join)
				r.Post("/join/{token}", meetupHandler.JoinByToken)
				r.Post("/{id}/leave", meetupHandler.Leave)
				r.Get("/my", meetupHandler.My)
			})
		})

		r.Group(func(r chi.Router) {
			r.Use(authMiddleware.AuthMiddleware(cfg))

			r.Get("/auth/me", authHandler.Me)

			r.Route("/profile", func(r chi.Router) {
				r.Get("/", profileHandler.GetMyProfile)
				r.Put("/", profileHandler.UpdateMyProfile)
				r.Get("/{id}", profileHandler.GetProfileByID)
			})
			r.Get("/geo/suggest", geoHandler.Suggest)

			r.Post("/files/upload", fileHandler.Upload)
		})

		r.Route("/chats", func(r chi.Router) {
			r.Use(authMiddleware.AuthMiddleware(cfg))

			r.Get("/", chatHandler.GetUserChats)
			r.Post("/{id}/messages", chatHandler.SendMessage)
			r.Get("/{id}/messages", chatHandler.GetMessages)
		})

		r.Route("/ws", func(r chi.Router) {
			r.Use(authMiddleware.AuthMiddleware(cfg))

			r.Get("/", wsHandler.ServeWs)
		})
	})

	router.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	return router
}
