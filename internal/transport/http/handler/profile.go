package handler

import (
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/service"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/dto"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/middleware"
	response "github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/response"
	"log/slog"
	"net/http"
	"strconv"
)

type ProfileHandler struct {
	service *service.ProfileService
	log     *slog.Logger
}

func NewProfileHandler(service *service.ProfileService, log *slog.Logger) *ProfileHandler {
	return &ProfileHandler{
		service: service, log: log}
}

// GetMyProfile
// @Summary      Получить мой профиль
// @Description  Возвращает профиль текущего пользователя вместе с тегами.
// @Tags         Profile
// @Produce      json
// @Success      200  {object}  dto.ProfileResponse
// @Failure      401  {object}  response.ErrorResponse
// @Failure      404  {object}  response.ErrorResponse
// @Security     BearerAuth
// @Router       /v1/profile [get]
func (h *ProfileHandler) GetMyProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		response.RespondError(w, service.ErrUnauthorized, h.log)
		return
	}

	profileDTO, err := h.service.GetProfile(r.Context(), userID)
	if err != nil {
		response.RespondError(w, err, h.log)
		return
	}

	response.JSON(w, http.StatusOK, profileDTO)
}

// GetProfileByID
// @Summary      Получить профиль пользователя
// @Description  Возвращает публичный профиль пользователя по его ID.
// @Tags         Profile
// @Produce      json
// @Param        id   path      int  true  "User ID"
// @Success      200  {object}  dto.ProfileResponse
// @Failure      400  {object}  response.ErrorResponse
// @Failure      401  {object}  response.ErrorResponse
// @Failure      404  {object}  response.ErrorResponse
// @Security     BearerAuth
// @Router       /v1/profile/{id} [get]
func (h *ProfileHandler) GetProfileByID(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	userID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		response.RespondError(w, service.ErrInvalidInput, h.log)
		return
	}

	profileDTO, err := h.service.GetProfile(r.Context(), userID)
	if err != nil {
		response.RespondError(w, err, h.log)
		return
	}

	response.JSON(w, http.StatusOK, profileDTO)
}

// UpdateMyProfile
// @Summary      Обновить мой профиль
// @Description  Обновляет никнейм, био, аватар и теги (передавать ID тегов).
// @Tags         Profile
// @Accept       json
// @Produce      json
// @Param        input body dto.UpdateProfileRequest true "Данные для обновления профиля"
// @Success      200  {object}  dto.ProfileResponse
// @Failure      400  {object}  response.ErrorResponse
// @Security     BearerAuth
// @Router       /v1/profile [put]
func (h *ProfileHandler) UpdateMyProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		response.RespondError(w, service.ErrUnauthorized, h.log)
		return
	}

	var req dto.UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.RespondError(w, service.ErrInvalidInput, h.log)
		return
	}

	updatedProfileDTO, err := h.service.UpdateProfile(r.Context(), userID, req)
	if err != nil {
		response.RespondError(w, err, h.log)
		return
	}

	response.JSON(w, http.StatusOK, updatedProfileDTO)
}
