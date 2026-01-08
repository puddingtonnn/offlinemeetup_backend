package handler

import (
	response "github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/response"
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
// @Success     200  {array}   dto.TagResponse
// @Failure     500  {object}  response.ErrorResponse
// @Router      /v1/tags [get]
func (h *TagHandler) List(w http.ResponseWriter, r *http.Request) {
	tags, err := h.service.ListTags(r.Context())
	if err != nil {
		response.RespondError(w, err, h.log)
		return
	}

	response.JSON(w, http.StatusOK, tags)
}
