package response

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/service"
)

type ErrorResponse struct {
	Error string `json:"error"`
}

func JSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload != nil {
		json.NewEncoder(w).Encode(payload)
	}
}

// RespondError - центральное место обработки ошибок
func RespondError(w http.ResponseWriter, err error, log *slog.Logger) {
	log.Error("Request failed", slog.String("err", err.Error()))

	var statusCode int
	var msg string

	switch {
	case errors.Is(err, service.ErrNotFound):
		statusCode = http.StatusNotFound
		msg = "Resource not found"
	case errors.Is(err, service.ErrAlreadyExists):
		statusCode = http.StatusConflict
		msg = "Resource already exists"
	case errors.Is(err, service.ErrForbidden):
		statusCode = http.StatusForbidden
		msg = "Access denied"
	case errors.Is(err, service.ErrUnauthorized):
		statusCode = http.StatusUnauthorized
		msg = "Unauthorized"
	case errors.Is(err, service.ErrInvalidInput):
		statusCode = http.StatusBadRequest
		msg = "Invalid input data"
	default:
		statusCode = http.StatusInternalServerError
		msg = "Internal Server Error"
	}

	RespondJSON(w, statusCode, ErrorResponse{Error: msg})
}
