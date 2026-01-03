package handler

import (
	"encoding/json"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/service"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/middleware"
	"net/http"
)

type ProfileHandler struct {
	service *service.ProfileService
}

func NewProfileHandler(service *service.ProfileService) *ProfileHandler {
	return &ProfileHandler{
		service: service,
	}
}

// GetMyProfile
// @Summary      Получить мой профиль
// @Description  Возвращает профиль текущего авторизованного пользователя. ID берется из JWT токена.
// @Tags         Profile
// @Produce      json
// @Success      200  {object}  domain.Profile
// @Failure      401  {string}  string "Unauthorized"
// @Failure      404  {string}  string "Profile not found"
// @Failure      500  {string}  string "Internal Server Error"
// @Security     ApiKeyAuth
// @Router       /v1/profile [get]
func (h *ProfileHandler) GetMyProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	profile, err := h.service.GetProfile(r.Context(), userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if profile == nil {
		http.Error(w, "Profile not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(profile)
}

// UpdateMyProfile
// @Summary      Обновить мой профиль
// @Description  Обновляет никнейм, био, аватар и теги пользователя.
// @Tags         Profile
// @Accept       json
// @Produce      json
// @Param        input body service.UpdateProfileInput true "Данные для обновления профиля"
// @Success      200  {object}  domain.Profile
// @Failure      400  {string}  string "Invalid input"
// @Failure      401  {string}  string "Unauthorized"
// @Failure      500  {string}  string "Internal Server Error"
// @Security     ApiKeyAuth
// @Router       /v1/profile [put]
func (h *ProfileHandler) UpdateMyProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var input service.UpdateProfileInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}

	updatedProfile, err := h.service.UpdateProfile(r.Context(), userID, &input)
	if err != nil {
		http.Error(w, "Failed to update profile", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updatedProfile)
}
