package recurring

import (
	"encoding/json"
	"net/http"

	"github.com/The-True-Hooha/stellance-backend/pkg/utils"
	"github.com/go-playground/validator/v10"
)

type RecurringHandler struct {
	service   *RecurringService
	validator *validator.Validate
}

func NewRecurringHandler(s *RecurringService) *RecurringHandler {
	return &RecurringHandler{service: s, validator: validator.New()}
}

// Create godoc
// @Summary      Create recurring invoice schedule
// @Tags         recurring
// @Accept       json
// @Produce      json
// @Param        body  body      CreateRecurringDTO  true  "Recurring invoice config"
// @Success      201   {object}  utils.ApiResponse
// @Failure      400   {object}  utils.ApiResponse
// @Security     BearerAuth
// @Router       /recurring [post]
func (h *RecurringHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := utils.GetUserIDFromContext(r.Context())
	if !ok {
		utils.WriteToJson(w, http.StatusUnauthorized, utils.ApiResponse{StatusCode: http.StatusUnauthorized, Message: "unauthorized"})
		return
	}
	var dto CreateRecurringDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{StatusCode: http.StatusBadRequest, Message: "invalid request body"})
		return
	}
	if err := h.validator.Struct(dto); err != nil {
		utils.HandleValidationError(w, err)
		return
	}
	resp := h.service.Create(r.Context(), userID, dto)
	utils.WriteToJson(w, resp.StatusCode, resp)
}

// List godoc
// @Summary      List recurring invoice schedules
// @Tags         recurring
// @Produce      json
// @Success      200  {object}  utils.ApiResponse
// @Failure      401  {object}  utils.ApiResponse
// @Security     BearerAuth
// @Router       /recurring [get]
func (h *RecurringHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := utils.GetUserIDFromContext(r.Context())
	if !ok {
		utils.WriteToJson(w, http.StatusUnauthorized, utils.ApiResponse{StatusCode: http.StatusUnauthorized, Message: "unauthorized"})
		return
	}
	resp := h.service.List(r.Context(), userID)
	utils.WriteToJson(w, resp.StatusCode, resp)
}

// Get godoc
// @Summary      Get recurring schedule by ID
// @Tags         recurring
// @Produce      json
// @Param        id  path  string  true  "Recurring ID"
// @Success      200  {object}  utils.ApiResponse
// @Failure      401  {object}  utils.ApiResponse
// @Security     BearerAuth
// @Router       /recurring/{id} [get]
func (h *RecurringHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID, ok := utils.GetUserIDFromContext(r.Context())
	if !ok {
		utils.WriteToJson(w, http.StatusUnauthorized, utils.ApiResponse{StatusCode: http.StatusUnauthorized, Message: "unauthorized"})
		return
	}
	id := r.PathValue("id")
	resp := h.service.Get(r.Context(), id, userID)
	utils.WriteToJson(w, resp.StatusCode, resp)
}

// Update godoc
// @Summary      Update recurring schedule
// @Tags         recurring
// @Accept       json
// @Produce      json
// @Param        id    path  string             true  "Recurring ID"
// @Param        body  body  UpdateRecurringDTO  true  "Fields to update"
// @Success      200   {object}  utils.ApiResponse
// @Failure      400   {object}  utils.ApiResponse
// @Security     BearerAuth
// @Router       /recurring/{id} [patch]
func (h *RecurringHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID, ok := utils.GetUserIDFromContext(r.Context())
	if !ok {
		utils.WriteToJson(w, http.StatusUnauthorized, utils.ApiResponse{StatusCode: http.StatusUnauthorized, Message: "unauthorized"})
		return
	}
	id := r.PathValue("id")
	var dto UpdateRecurringDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{StatusCode: http.StatusBadRequest, Message: "invalid request body"})
		return
	}
	resp := h.service.Update(r.Context(), id, userID, dto)
	utils.WriteToJson(w, resp.StatusCode, resp)
}

// Delete godoc
// @Summary      Delete recurring schedule
// @Tags         recurring
// @Produce      json
// @Param        id  path  string  true  "Recurring ID"
// @Success      200  {object}  utils.ApiResponse
// @Failure      401  {object}  utils.ApiResponse
// @Security     BearerAuth
// @Router       /recurring/{id} [delete]
func (h *RecurringHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := utils.GetUserIDFromContext(r.Context())
	if !ok {
		utils.WriteToJson(w, http.StatusUnauthorized, utils.ApiResponse{StatusCode: http.StatusUnauthorized, Message: "unauthorized"})
		return
	}
	id := r.PathValue("id")
	resp := h.service.Delete(r.Context(), id, userID)
	utils.WriteToJson(w, resp.StatusCode, resp)
}
