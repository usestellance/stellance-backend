package notifications

import (
	"net/http"
	"strconv"

	"github.com/The-True-Hooha/stellance-backend/pkg/utils"
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

// GetNotifications godoc
// @Summary      Get user notifications
// @Tags         notifications
// @Produce      json
// @Param        page     query  int     false  "Page number"
// @Param        count    query  int     false  "Items per page"
// @Param        order_by query  string  false  "ASC or DESC"
// @Param        viewed   query  bool    false  "Filter by viewed status"
// @Success      200  {object}  utils.ApiResponse
// @Failure      401  {object}  utils.ApiResponse
// @Security     BearerAuth
// @Router       /notifications [get]
func (h *NotificationHandler) GetNotifications(w http.ResponseWriter, r *http.Request) {
	log := h.service.log
	ctx := r.Context()

	userId, ok := utils.GetUserIDFromContext(ctx)
	if !ok {
		log.Error("unauthorized access")
		http.Error(w, "invalid request! not allowed", http.StatusUnauthorized)
		return
	}

	dto := GetNotificationsQuery{}

	page := r.URL.Query().Get("page")
	if page != "" {
		if p, err := strconv.Atoi(page); err == nil {
			dto.Page = p
		}
	} else {
		dto.Page = 1
	}

	count := r.URL.Query().Get("count")
	if count != "" {
		if c, err := strconv.Atoi(count); err == nil {
			dto.Count = c
		}
	} else {
		dto.Count = 15
	}

	orderBy := r.URL.Query().Get("order_by")
	if orderBy == "" {
		dto.OrderBy = utils.OrderByDESC
	} else {
		dto.OrderBy = utils.OrderByType(orderBy)
	}

	viewed := r.URL.Query().Get("viewed")
	var checkView bool
	if viewed != "" {
		v, err := strconv.ParseBool(viewed)
		if err != nil {
			http.Error(w, "Invalid approve value", http.StatusBadRequest)
			return
		}
		checkView = v
		dto.Viewed = &checkView
	}

	response := h.service.GetUserNotifications(ctx, userId, dto)
	utils.WriteToJson(w, response.StatusCode, response)
}

// EditNotificationViewStatus godoc
// @Summary      Mark notification as viewed/unviewed
// @Tags         notifications
// @Produce      json
// @Param        id      path   string  true  "Notification ID"
// @Param        viewed  query  bool    true  "Viewed status"
// @Success      200  {object}  utils.ApiResponse
// @Failure      401  {object}  utils.ApiResponse
// @Security     BearerAuth
// @Router       /notifications/{id} [patch]
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

// DeleteNotificationById godoc
// @Summary      Delete a notification
// @Tags         notifications
// @Produce      json
// @Param        id  path  string  true  "Notification ID"
// @Success      200  {object}  utils.ApiResponse
// @Failure      401  {object}  utils.ApiResponse
// @Security     BearerAuth
// @Router       /notifications/{id} [delete]
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

// GetNotificationById godoc
// @Summary      Get notification by ID
// @Tags         notifications
// @Produce      json
// @Param        id  path  string  true  "Notification ID"
// @Success      200  {object}  utils.ApiResponse
// @Failure      401  {object}  utils.ApiResponse
// @Security     BearerAuth
// @Router       /notifications/{id} [get]
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
