package handler

import (
	"encoding/json"
	"net/http"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/service"
)

type TagHandler struct {
	service *service.TagService
}

func NewTagHandler(service *service.TagService) *TagHandler {
	return &TagHandler{service: service}
}

// List
// @Summary     Получить список тегов
// @Description Возвращает справочник всех доступных тегов/интересов.
// @Tags        Tags
// @Produce     json
// @Success     200  {array}   domain.Tag
// @Failure     500  {string}  string  "Internal Server Error"
// @Router      /v1/tags [get]
func (h *TagHandler) List(w http.ResponseWriter, r *http.Request) {
	tags, err := h.service.ListTags(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tags)
}
