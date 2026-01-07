package handler

import (
	"encoding/json"
	transport "github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/response"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/service"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/dto"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/middleware"
)

type MeetupHandler struct {
	service *service.MeetupService
	log     *slog.Logger
}

func NewMeetupHandler(service *service.MeetupService, log *slog.Logger) *MeetupHandler {
	return &MeetupHandler{service: service, log: log}
}

// CreateMeetup
// @Summary Создать митап
// @Security BearerAuth
// @Tags 	Meetups
// @Accept	json
// @Produce	json
// @Param input body dto.CreateMeetupRequest true "Данные митапа"
// @Success 201
// @Failure 400 {string} string "Error"
// @Router /v1/meetups [post]
func (h *MeetupHandler) CreateMeetup(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		transport.RespondError(w, service.ErrUnauthorized, h.log)
		return
	}

	var req dto.CreateMeetupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		transport.RespondError(w, service.ErrInvalidInput, h.log)
		return
	}

	resp, err := h.service.CreateMeetup(r.Context(), userID, req)
	if err != nil {
		transport.RespondError(w, err, h.log)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

// GetByID
// @Summary     Получить митап по ID
// @Description Возвращает детальную информацию о митапе.
// @Security    BearerAuth
// @Tags        Meetups
// @Produce     json
// @Param       id   path      int  true  "Meetup ID"
// @Success     200  {object}  dto.MeetupResponse
// @Failure     400  {string}  string  "Invalid ID"
// @Failure     404  {string}  string  "Meetup not found"
// @Failure     500  {string}  string  "Internal Server Error"
// @Router      /v1/meetups/{id} [get]
func (h *MeetupHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)

	if err != nil {
		transport.RespondError(w, service.ErrInvalidInput, h.log)
	}

	resp, err := h.service.GetMeetup(r.Context(), id)
	if err != nil {
		transport.RespondError(w, err, h.log)
		return
	}

	json.NewEncoder(w).Encode(resp)
}

// List
// @Summary     Поиск и список митапов
// @Description Возвращает список митапов. Если переданы lat/lng/radius — ищет ближайшие. Иначе сортирует по времени.
// @Security    BearerAuth
// @Tags        Meetups
// @Produce     json
// @Param       lat     query     number  false  "Широта (Latitude)"
// @Param       lng     query     number  false  "Долгота (Longitude)"
// @Param       radius  query     int     false  "Радиус поиска (в метрах)"
// @Param       limit   query     int     false  "Лимит записей (default: 20)"
// @Param       offset  query     int     false  "Смещение (pagination)"
// @Success     200     {array}   dto.MeetupResponse
// @Failure     500     {string}  string  "Internal Server Error"
// @Router      /v1/meetups [get]
func (h *MeetupHandler) List(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	lat, _ := strconv.ParseFloat(query.Get("lat"), 64)
	lng, _ := strconv.ParseFloat(query.Get("lng"), 64)
	radius, _ := strconv.Atoi(query.Get("radius"))
	limit, _ := strconv.Atoi(query.Get("limit"))
	offset, _ := strconv.Atoi(query.Get("offset"))

	if lat != 0 && lng != 0 && radius == 0 {
		radius = 5000 // 5 км
	}

	if limit <= 0 || limit > 100 {
		limit = 20
	}

	filter := dto.MeetupFilter{
		Lat:    lat,
		Lng:    lng,
		Radius: radius,
		Limit:  limit,
		Offset: offset,
	}

	list, err := h.service.ListMeetups(r.Context(), filter)
	if err != nil {
		transport.RespondError(w, err, h.log)
		return
	}

	json.NewEncoder(w).Encode(list)
}

// Update
// @Summary     Обновить митап
// @Description Частичное обновление полей митапа. Требует прав создателя (owner).
// @Security    BearerAuth
// @Tags        Meetups
// @Accept      json
// @Produce     json
// @Param       id     path      int                      true  "Meetup ID"
// @Param       input  body      dto.UpdateMeetupRequest  true  "Поля для обновления (nil поля игнорируются)"
// @Success     200    {object}  dto.MeetupResponse
// @Failure     400    {string}  string  "Invalid input"
// @Failure     403    {string}  string  "Forbidden: You are not the owner"
// @Failure     404    {string}  string  "Meetup not found"
// @Failure     500    {string}  string  "Internal Server Error"
// @Router      /v1/meetups/{id} [put]
func (h *MeetupHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		transport.RespondError(w, service.ErrUnauthorized, h.log)
		return
	}

	idStr := chi.URLParam(r, "id")
	meetupID, _ := strconv.ParseInt(idStr, 10, 64)

	var req dto.UpdateMeetupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		transport.RespondError(w, service.ErrInvalidInput, h.log)
		return
	}

	resp, err := h.service.UpdateMeetup(r.Context(), userID, meetupID, req)
	if err != nil {
		transport.RespondError(w, err, h.log)
		return
	}

	json.NewEncoder(w).Encode(resp)
}

// Delete
// @Summary     Удалить митап
// @Description Удаляет митап (Soft Delete). Требует прав создателя (owner).
// @Security    BearerAuth
// @Tags        Meetups
// @Produce     json
// @Param       id   path      int  true  "Meetup ID"
// @Success     204  {object}  nil  "No Content"
// @Failure     400  {string}  string  "Invalid ID"
// @Failure     403  {string}  string  "Forbidden: You are not the owner"
// @Failure     404  {string}  string  "Meetup not found"
// @Failure     500  {string}  string  "Internal Server Error"
// @Router      /v1/meetups/{id} [delete]
func (h *MeetupHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		transport.RespondError(w, service.ErrUnauthorized, h.log)
		return
	}

	idStr := chi.URLParam(r, "id")
	meetupID, _ := strconv.ParseInt(idStr, 10, 64)

	err := h.service.DeleteMeetup(r.Context(), userID, meetupID)
	if err != nil {
		transport.RespondError(w, err, h.log)
	}
	w.WriteHeader(http.StatusNoContent)
}
