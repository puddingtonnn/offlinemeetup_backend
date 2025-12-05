package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/handler"
	authMiddleware "github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/middleware"
)

func NewRouter(authHandler *handler.AuthHandler) *chi.Mux {
	router := chi.NewRouter()

	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)

	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	router.Group(func(r chi.Router) {
		r.Post("/auth/register", authHandler.Register)
		r.Post("/auth/login", authHandler.Login)
	})

	router.Group(func(r chi.Router) {
		r.Use(authMiddleware.AuthMiddleware)
		r.Get("/auth/me", authHandler.Me)
	})

	return router
}
