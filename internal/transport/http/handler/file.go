package handler

import (
	"log/slog"
	"net/http"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/service"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/middleware"
	response "github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/response"
)

type FileHandler struct {
	service *service.FileService
	log     *slog.Logger
}

func NewFileHandler(service *service.FileService, log *slog.Logger) *FileHandler {
	return &FileHandler{service: service, log: log}
}

// Upload
// @Summary Загрузить файл
// @Security BearerAuth
// @Tags 	Files
// @Accept	multipart/form-data
// @Produce	json
// @Param   file formData file true "Файл"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} response.ErrorResponse
// @Router /v1/files/upload [post]
func (h *FileHandler) Upload(w http.ResponseWriter, r *http.Request) {
	// Жесткий лимит 10MB на весь запрос
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		response.RespondError(w, service.ErrInvalidInput, h.log)
		return
	}

	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		response.RespondError(w, service.ErrUnauthorized, h.log)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		response.RespondError(w, service.ErrInvalidInput, h.log)
		return
	}
	defer file.Close()

	res, err := h.service.Upload(r.Context(), userID, header.Filename, header.Header.Get("Content-Type"), header.Size, file)
	if err != nil {
		response.RespondError(w, err, h.log)
		return
	}

	response.JSON(w, http.StatusCreated, map[string]interface{}{
		"id": res.ID,
	})
}
