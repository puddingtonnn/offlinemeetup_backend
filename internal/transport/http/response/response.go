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

// ValidationErrorResponse возвращает 400 с детализацией по полям.
type ValidationErrorResponse struct {
	Error  string            `json:"error"`
	Fields map[string]string `json:"fields"`
}

// RespondValidation отправляет 400 с картой ошибок по конкретным полям.
func RespondValidation(w http.ResponseWriter, fields map[string]string) {
	JSON(w, http.StatusBadRequest, ValidationErrorResponse{
		Error:  "validation failed",
		Fields: fields,
	})
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
	case errors.Is(err, service.ErrMeetupFinished):
		statusCode = http.StatusConflict
		msg = "Meetup is finished or cancelled"
	case errors.Is(err, service.ErrOrganizerCannotLeave):
		statusCode = http.StatusConflict
		msg = "Organizer cannot leave own meetup"
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

	JSON(w, statusCode, ErrorResponse{Error: msg})
}
