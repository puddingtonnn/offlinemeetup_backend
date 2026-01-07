package handler

import (
	"encoding/json"
	transport "github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/response"
	"log/slog"
	"net/http"

	"github.com/puddingtonnn/offlinemeetup_backend/internal/service"
)

type TagHandler struct {
	service *service.TagService
	log     *slog.Logger
}

func NewTagHandler(service *service.TagService, log *slog.Logger) *TagHandler {
	return &TagHandler{service: service, log: log}
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
		transport.RespondError(w, err, h.log)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tags)
}
