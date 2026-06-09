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

// ClearRedisHandler godoc
// @Summary      Clear Redis cache (admin only)
// @Tags         auth
// @Produce      json
// @Success      200  {object}  utils.ApiResponse
// @Failure      403  {object}  utils.ApiResponse
// @Security     BearerAuth
// @Router       /auth/clear [post]
func (h *AuthHandler) ClearRedisHandler(w http.ResponseWriter, r *http.Request) {
	data := h.service.ClearRedis(r.Context())
	utils.WriteToJson(w, data.StatusCode, data)
}

// SignUpHandler godoc
// @Summary      Register a new user
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      AuthRequestDto  true  "Sign up credentials"
// @Success      201   {object}  utils.ApiResponse
// @Failure      400   {object}  utils.ApiResponse
// @Router       /auth/signup [post]
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

// AdminRegister godoc
// @Summary      Register a new admin user
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      AuthRequestDto  true  "Admin credentials"
// @Success      201   {object}  utils.ApiResponse
// @Failure      400   {object}  utils.ApiResponse
// @Router       /auth/admin/signup [post]
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

// LoginHandler godoc
// @Summary      Login
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      AuthRequestDto  true  "Login credentials"
// @Success      200   {object}  utils.ApiResponse
// @Failure      400   {object}  utils.ApiResponse
// @Failure      401   {object}  utils.ApiResponse
// @Router       /auth/login [post]
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

// RefreshTokenHandler godoc
// @Summary      Refresh access token
// @Tags         auth
// @Produce      json
// @Success      200  {object}  utils.ApiResponse
// @Failure      401  {object}  utils.ApiResponse
// @Security     BearerAuth
// @Router       /auth/token [post]
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

// ValidateEmailHandler godoc
// @Summary      Validate email address
// @Tags         auth
// @Produce      json
// @Param        token  query  string  true   "Verification token"
// @Param        email  query  string  false  "Email address"
// @Success      200    {object}  utils.ApiResponse
// @Failure      400    {object}  utils.ApiResponse
// @Router       /auth/validate [get]
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

// ResendEmailVerification godoc
// @Summary      Resend email verification
// @Tags         auth
// @Produce      json
// @Param        email  query  string  true  "Email address"
// @Success      200    {object}  utils.ApiResponse
// @Failure      400    {object}  utils.ApiResponse
// @Router       /auth/resend-email [get]
func (h *AuthHandler) ResendEmailVerification(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	if email == "" {
		http.Error(w, "invalid request! auth email not found", http.StatusBadRequest)
		return
	}

	data := h.service.ResendEmail(r.Context(), email)
	utils.WriteToJson(w, data.StatusCode, data)
}

// RequestPasswordReset godoc
// @Summary      Request password reset email
// @Tags         auth
// @Produce      json
// @Param        email  query  string  true  "Email address"
// @Success      200    {object}  utils.ApiResponse
// @Failure      400    {object}  utils.ApiResponse
// @Router       /auth/reset [get]
func (h *AuthHandler) RequestPasswordReset(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	if email == "" {
		http.Error(w, "invalid request! auth email not found", http.StatusBadRequest)
		return
	}
	data := h.service.RequestPasswordReset(r.Context(), email)
	utils.WriteToJson(w, data.StatusCode, data)
}

// ChangePassword godoc
// @Summary      Change password (authenticated)
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      ChangePasswordDTO  true  "Old and new passwords"
// @Success      200   {object}  utils.ApiResponse
// @Failure      401   {object}  utils.ApiResponse
// @Security     BearerAuth
// @Router       /auth/change-password [post]
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, ok := utils.GetUserIDFromContext(ctx)
	if !ok {
		http.Error(w, "unauthorized here", http.StatusUnauthorized)
		return
	}

	var dto ChangePasswordDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	data := h.service.ChangeUserPassword(ctx, dto, id)
	utils.WriteToJson(w, data.StatusCode, data)
}

// UpdatePassword godoc
// @Summary      Reset password via token
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      ResetPasswordDto  true  "Reset token and new password"
// @Success      200   {object}  utils.ApiResponse
// @Failure      400   {object}  utils.ApiResponse
// @Router       /auth/reset-password [post]
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

// SocialSignUpHandler godoc
// @Summary      Social auth (Google / GitHub)
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      ProviderLogin  true  "Provider token payload"
// @Success      200   {object}  utils.ApiResponse
// @Failure      400   {object}  utils.ApiResponse
// @Router       /auth/social [post]
func (h *AuthHandler) SocialSignUpHandler(w http.ResponseWriter, r *http.Request) {
	var dto ProviderLogin

	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	data := h.service.HandleSocialAuth(r.Context(), dto)
	utils.WriteToJson(w, data.StatusCode, data)
}
