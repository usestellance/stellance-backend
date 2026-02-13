package auth

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/The-True-Hooha/stellance-backend/internal/user"
	"github.com/The-True-Hooha/stellance-backend/pkg/utils"
	"github.com/go-playground/validator/v10"
)

type AuthHandler struct {
	service   *AuthServiceConfig
	validator *validator.Validate
}

func NewAuthHandler(config *AuthServiceConfig) *AuthHandler {
	v := validator.New()
	v.RegisterValidation("passwd", utils.ValidatePassword)
	return &AuthHandler{
		service:   config,
		validator: v,
	}
}

func (h *AuthHandler) ClearRedisHandler(w http.ResponseWriter, r *http.Request) {
	data := h.service.ClearRedis(r.Context())

	utils.WriteToJson(w, data.StatusCode, data)
}

func (handler *AuthHandler) SignUpHandler(w http.ResponseWriter, r *http.Request) {
	var dto AuthRequestDto
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if err := handler.validator.Struct(dto); err != nil {
		utils.HandleValidationError(w, err)
		return
	}

	if valid, errMsg := utils.CheckPasswordRequirements(dto.Password); !valid {
		http.Error(w, errMsg, http.StatusBadRequest)
		return
	}

	data := handler.service.CreateNewUser(r.Context(), dto, user.RoleUser)
	utils.WriteToJson(w, data.StatusCode, data)
}

func (handler *AuthHandler) AdminRegister(w http.ResponseWriter, r *http.Request) {
	var dto AuthRequestDto
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if err := handler.validator.Struct(dto); err != nil {
		utils.HandleValidationError(w, err)
		return
	}

	data := handler.service.CreateNewUser(r.Context(), dto, user.RoleAdmin)
	utils.WriteToJson(w, data.StatusCode, data)
}

func (h *AuthHandler) LoginHandler(w http.ResponseWriter, r *http.Request) {
	var dto AuthRequestDto
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.validator.Struct(dto); err != nil {
		utils.HandleValidationError(w, err)
		return
	}

	data := h.service.Login(r.Context(), dto)
	utils.WriteToJson(w, data.StatusCode, data)
}

func (h *AuthHandler) RefreshTokenHandler(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		http.Error(w, "missing or malformed Authorization header", http.StatusUnauthorized)
		return
	}

	accessToken := strings.TrimPrefix(authHeader, "Bearer ")
	data := h.service.GenerateRefreshToken(r.Context(), accessToken)
	utils.WriteToJson(w, data.StatusCode, data)
}

func (h *AuthHandler) ValidateEmailHandler(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	email_ := r.URL.Query().Get("email")
	if token == "" {
		http.Error(w, "invalid request! auth token not found", http.StatusBadRequest)
		return
	}

	data := h.service.ValidateEmail(r.Context(), token, email_)
	utils.WriteToJson(w, data.StatusCode, data)
}

func (h *AuthHandler) ResendEmailVerification(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	if email == "" {
		http.Error(w, "invalid request! auth email not found", http.StatusBadRequest)
		return
	}

	data := h.service.ResendEmail(r.Context(), email)
	utils.WriteToJson(w, data.StatusCode, data)
}

func (h *AuthHandler) RequestPasswordReset(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	if email == "" {
		http.Error(w, "invalid request! auth email not found", http.StatusBadRequest)
		return
	}
	data := h.service.RequestPasswordReset(r.Context(), email)
	utils.WriteToJson(w, data.StatusCode, data)
}

func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, ok := utils.GetUserIDFromContext(ctx)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}
	
	var dto ChangePasswordDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
	}

	data := h.service.ChangeUserPassword(ctx, dto, id)

	utils.WriteToJson(w, data.StatusCode, data)	
}

func (h *AuthHandler) UpdatePassword(w http.ResponseWriter, r *http.Request) {
	var dto ResetPasswordDto
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if err := h.validator.Struct(dto); err != nil {
		utils.HandleValidationError(w, err)
		return
	}

	if valid, errMsg := utils.CheckPasswordRequirements(dto.Password); !valid {
		http.Error(w, errMsg, http.StatusBadRequest)
		return
	}

	data := h.service.ResetPassword(r.Context(), dto)
	utils.WriteToJson(w, data.StatusCode, data)
}
