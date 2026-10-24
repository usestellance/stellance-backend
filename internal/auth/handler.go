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
// @Description  Flush the Redis cache (invoice and search caches, email tokens). Admin only; use sparingly.
// @Tags         auth
// @Produce      json
// @Success      200  {object}  utils.ApiResponse  "Cache cleared"
// @Failure      401  {object}  utils.ApiResponse  "Missing or invalid bearer token"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
// @Security     BearerAuth
// @Router       /auth/clear [post]
func (h *AuthHandler) ClearRedisHandler(w http.ResponseWriter, r *http.Request) {
	data := h.service.ClearRedis(r.Context())
	utils.WriteToJson(w, data.StatusCode, data)
}

// SignUpHandler godoc
// @Summary      Register a new user
// @Description  Create a password account and send a verification email. The user cannot log in until the email is verified.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      AuthRequestDto  true  "Sign up credentials"
// @Success      201  {object}  utils.ApiResponse{data=AuthResponseDto}  "Account created"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      403  {object}  utils.ApiResponse  "Email already registered"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
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

// AdminRegister is not routed; admins are created out of band.
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
// @Description  Email and password sign-in. Unverified accounts get 403 with a fresh verification email and no token. Returns an access token valid for 1 hour.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      AuthRequestDto  true  "Login credentials"
// @Success      200  {object}  utils.ApiResponse{data=AuthLoginResponseDto}  "Signed in"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      403  {object}  utils.ApiResponse  "Email not verified (data.email_verified=false), or wrong password"
// @Failure      404  {object}  utils.ApiResponse  "Unknown email"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
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
// @Description  Exchange the current access token for a 7-day refresh token.
// @Tags         auth
// @Produce      json
// @Success      200  {object}  utils.ApiResponse{data=RefreshTokenResult}  "Refresh token issued"
// @Failure      401  {object}  utils.ApiResponse  "Missing or invalid bearer token"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
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
// @Description  Confirm an email address with the token from the verification email. An expired token triggers a new email.
// @Tags         auth
// @Produce      json
// @Param        token  query  string  true   "Verification token"
// @Param        email  query  string  false  "Email address"
// @Success      200  {object}  utils.ApiResponse  "Email verified"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
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
// @Description  Send the verification email again.
// @Tags         auth
// @Produce      json
// @Param        email  query  string  true  "Email address"
// @Success      200  {object}  utils.ApiResponse  "Email sent"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
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
// @Description  Email a password reset link to the address, if an account exists.
// @Tags         auth
// @Produce      json
// @Param        email  query  string  true  "Email address"
// @Success      200  {object}  utils.ApiResponse  "Reset email sent"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
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
// @Description  Change the password of the signed-in user. Requires the old password.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      ChangePasswordDTO  true  "Old and new passwords"
// @Success      200  {object}  utils.ApiResponse{data=PasswordChangedResult}  "Password changed"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      401  {object}  utils.ApiResponse  "Missing or invalid bearer token"
// @Failure      403  {object}  utils.ApiResponse  "Old password is wrong"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
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
// @Description  Set a new password using the token from the reset email.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      ResetPasswordDto  true  "Reset token and new password"
// @Success      200  {object}  utils.ApiResponse  "Password updated"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
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
// @Summary      Google sign-in; the server verifies the Google ID token
// @Description  Google sign-in. The server verifies the Google ID token (RS256 signature, issuer, audience = GOOGLE_CLIENT_ID, expiry, verified email) and ignores any email or user id sent by the client. Creates an email-verified account on first use.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      ProviderLogin  true  "Provider token payload"
// @Success      200  {object}  utils.ApiResponse{data=AuthLoginResponseDto}  "Signed in (new accounts get SocialSignupResult)"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      401  {object}  utils.ApiResponse  "Invalid Google token"
// @Failure      403  {object}  utils.ApiResponse  "Account uses password login"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
// @Failure      503  {object}  utils.ApiResponse  "Google keys unreachable"
// @Router       /auth/social [post]
func (h *AuthHandler) SocialSignUpHandler(w http.ResponseWriter, r *http.Request) {
	var dto ProviderLogin

	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if dto.IDToken == "" {
		http.Error(w, "id_token is required", http.StatusBadRequest)
		return
	}

	data := h.service.HandleSocialAuth(r.Context(), dto)
	utils.WriteToJson(w, data.StatusCode, data)
}
