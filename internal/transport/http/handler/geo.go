package handler

import (
	"github.com/puddingtonnn/offlinemeetup_backend/internal/service"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/response"
	"net/http"
)

type GeoHandler struct {
	service *service.GeoService
}

func NewGeoHandler(service *service.GeoService) *GeoHandler {
	return &GeoHandler{service: service}
}

// Suggest
// @Summary      Подсказки адресов (DaData)
// @Description  Ищет адреса и возвращает координаты
// @Tags         Geo
// @Security     BearerAuth
// @Param        address_part   query     string  true  "Часть адреса (например: москва лен)"
// @Success      200 {array}   dto.AddressSuggestion
// @Router       /v1/geo/suggest [get]
func (h *GeoHandler) Suggest(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("address_part")

	if len(query) < 2 {
		response.JSON(w, http.StatusOK, []interface{}{})
		return
	}

	suggestions, err := h.service.SuggestAddress(query)
	if err != nil {
		http.Error(w, "Geo provider error", http.StatusInternalServerError)
		return
	}

	response.JSON(w, http.StatusOK, suggestions)
}
