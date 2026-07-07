package handler

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/dto"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/service"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/response"
)

type GeoHandler struct {
	service *service.GeoService
	log     *slog.Logger
}

func NewGeoHandler(service *service.GeoService, log *slog.Logger) *GeoHandler {
	return &GeoHandler{service: service, log: log}
}

// Suggest
// @Summary      Подсказки адресов (DaData)
// @Description  Ищет адреса и возвращает координаты
// @Tags         Geo
// @Security     BearerAuth
// @Param        address_part   query     string  false  "Часть адреса (например: москва лен)"
// @Param		 lat			query	  string  false  "Широта"
// @Param 		 lon			query	  string  false	 "Долгота"
// @Success      200 {array}   dto.AddressSuggestion
// @Router       /v1/geo/suggest [get]
func (h *GeoHandler) Suggest(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	latStr := query.Get("lat")
	lonStr := query.Get("lon")
	addressPart := query.Get("address_part")

	var suggestions []dto.AddressSuggestion
	var err error

	if latStr != "" || lonStr != "" {
		lat, errLat := strconv.ParseFloat(latStr, 64)
		lon, errLon := strconv.ParseFloat(lonStr, 64)

		if errLat != nil || errLon != nil {
			response.RespondError(w, service.ErrInvalidInput, h.log)
			return
		}

		suggestions, err = h.service.SuggestByGeo(r.Context(), lat, lon)
	} else if len(addressPart) >= 2 {
		suggestions, err = h.service.SuggestAddress(r.Context(), addressPart)
	} else {
		response.JSON(w, http.StatusOK, []any{})
		return
	}

	if err != nil {
		response.RespondError(w, service.ErrInternal, h.log)
		return
	}

	response.JSON(w, http.StatusOK, suggestions)
}
