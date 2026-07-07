package handler

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/service"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/middleware"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/response"
)

// requireUserID reads the authenticated user ID from the request context. When
// it is absent (no/invalid Bearer token on an auth-required route) it writes a
// 401 and returns ok=false, so a caller just does `if _, ok := ...; !ok { return }`.
func requireUserID(w http.ResponseWriter, r *http.Request, log *slog.Logger) (int64, bool) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		response.RespondError(w, service.ErrUnauthorized, log)
	}
	return userID, ok
}

// pathInt64 parses an int64 URL path parameter. On a malformed value it writes a
// 400 through the ErrInvalidInput sentinel and returns ok=false. Using the
// sentinel matters: a bare fmt.Errorf would miss RespondError's errors.Is switch
// and fall through to a 500 for what is a client mistake.
func pathInt64(w http.ResponseWriter, r *http.Request, name string, log *slog.Logger) (int64, bool) {
	v, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	if err != nil {
		response.RespondError(w, service.ErrInvalidInput, log)
		return 0, false
	}
	return v, true
}
