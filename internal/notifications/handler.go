package notifications

import (
	"net/http"
	"strconv"

	"github.com/The-True-Hooha/stellance-backend.git/pkg/utils"
	"github.com/go-playground/validator/v10"
)

type NotificationHandler struct {
	service   *NotificationService
	validator *validator.Validate
}

func NewNotificationHandler(ns *NotificationService) *NotificationHandler {
	v := validator.New()

	return &NotificationHandler{
		service:   ns,
		validator: v,
	}
}

func (h *NotificationHandler) GetNotifications(w http.ResponseWriter, r *http.Request) {
	log := h.service.log
	ctx := r.Context()

	viewedStr := r.URL.Query().Get("viewed")

	userId, ok := utils.GetUserIDFromContext(ctx)
	if !ok {
		log.Error("unauthorized access")
		http.Error(w, "invalid request! not allowed", http.StatusUnauthorized)
		return
	}

	viewed, err := strconv.ParseBool(viewedStr)
	if err != nil {
		http.Error(w, "Invalid approve value", http.StatusBadRequest)
		return
	}

	response := h.service.GetUserNotifications(ctx, userId, &viewed)
	utils.WriteToJson(w, response.StatusCode, response)
}

func (h *NotificationHandler) EditNotificationViewStatus(w http.ResponseWriter, r *http.Request) {
	log := h.service.log
	ctx := r.Context()

	userId, ok := utils.GetUserIDFromContext(ctx)
	if !ok {
		log.Error("unauthorized access")
		http.Error(w, "invalid request! not allowed", http.StatusUnauthorized)
		return
	}
	notificationId := r.PathValue("id")

	viewedStr := r.URL.Query().Get("viewed")

	viewed, err := strconv.ParseBool(viewedStr)
	if err != nil {
		http.Error(w, "Invalid approve value", http.StatusBadRequest)
		return
	}

	response := h.service.UpdateNotificationViewStatus(ctx, notificationId, userId, viewed)
	utils.WriteToJson(w, response.StatusCode, response)
}

func (h *NotificationHandler) DeleteNotificationById(w http.ResponseWriter, r *http.Request) {
	log := h.service.log
	ctx := r.Context()

	userId, ok := utils.GetUserIDFromContext(ctx)
	if !ok {
		log.Error("unauthorized access")
		http.Error(w, "invalid request! not allowed", http.StatusUnauthorized)
		return
	}
	notificationId := r.PathValue("id")
	response := h.service.DeleteNotificationByID(ctx, notificationId, userId)
	utils.WriteToJson(w, response.StatusCode, response)
}

func (h *NotificationHandler) GetNotificationById(w http.ResponseWriter, r *http.Request) {
	log := h.service.log
	ctx := r.Context()

	userId, ok := utils.GetUserIDFromContext(ctx)
	if !ok {
		log.Error("unauthorized access")
		http.Error(w, "invalid request! not allowed", http.StatusUnauthorized)
		return
	}
	notificationId := r.PathValue("id")
	response := h.service.GetNotificationByID(ctx, notificationId, userId)
	utils.WriteToJson(w, response.StatusCode, response)
}
