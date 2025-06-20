package user

import (
	"encoding/json"
	"net/http"

	"github.com/The-True-Hooha/stellance-backend.git/pkg/utils"
	"github.com/go-playground/validator/v10"
)

type UserHandler struct {
	service   *UserService
	validator *validator.Validate
}

func NewUserHandler(h *UserService) *UserHandler {
	return &UserHandler{
		service:   h,
		validator: validator.New(),
	}
}

func (h *UserHandler) CompleteProfileHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	email, ok := utils.GetUserEmailFromContext(ctx)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}

	var dto CompleteProfileRequestDto
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if err := h.validator.Struct(dto); err != nil {
		utils.HandleValidationError(w, err)
		return
	}
	response := h.service.CompleteUserProfile(ctx, email, dto)
	utils.WriteToJson(w, response.StatusCode, response)
}

func (h *UserHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.PathValue("id")
	requestingUserID, ok := utils.GetUserIDFromContext(ctx)
	if !ok {
		http.Error(w, "Unauthorized: missing user ID", http.StatusUnauthorized)
		return
	}
	role, ok := utils.GetRoleFromContext(ctx)
	if !ok {
		http.Error(w, "Unauthorized: missing role", http.StatusUnauthorized)
		return
	}
	if userID != requestingUserID && role != string(RoleAdmin) {
		http.Error(w, "Forbidden: not allowed to access this profile", http.StatusForbidden)
		return
	}
	response := h.service.GetProfileByID(ctx, userID, requestingUserID)
	utils.WriteToJson(w, response.StatusCode, response)
}

func (h *UserHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := h.service.log
	userID, ok := utils.GetUserIDFromContext(ctx)
	if !ok {
		http.Error(w, "login to edit profile", http.StatusUnauthorized)
		return
	}
	var dto UpdateProfileDto
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		log.Error("failed to decode request", "error", err)
		http.Error(w, "not allowed", http.StatusForbidden)
		return
	}
	if dto.FirstName == nil && dto.LastName == nil &&
		dto.BusinessName == nil && dto.PhoneNumber == nil && dto.Country == nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if err := h.validator.Struct(dto); err != nil {
		utils.HandleValidationError(w, err)
		return
	}
	response := h.service.UpdateProfile(ctx, userID, dto)
	utils.WriteToJson(w, response.StatusCode, response)
}
