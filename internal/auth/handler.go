package auth

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/The-True-Hooha/stellance-backend.git/pkg/utils"
	"github.com/go-playground/validator/v10"
)

var validate = validator.New()

func (config *AuthServiceConfig) SignUpHandler(w http.ResponseWriter, r *http.Request) {
	var dto AuthRequestDto
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if err := validate.Struct(dto); err != nil {
		utils.HandleValidationError(w, err)
		return
	}

	data := NewAuthService().CreateNewUser(r.Context(), dto)
	utils.WriteToJson(w, http.StatusCreated, data)
}

func (config *AuthServiceConfig) LoginHandler(w http.ResponseWriter, r *http.Request) {
	var dto AuthRequestDto

	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := validate.Struct(dto); err != nil {
		utils.HandleValidationError(w, err)
		return
	}

	data := NewAuthService().Login(r.Context(), dto)
	utils.WriteToJson(w, http.StatusOK, data)
}

func (config *AuthServiceConfig) RefreshTokenHandler(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		http.Error(w, "missing or malformed Authorization header", http.StatusUnauthorized)
		return
	}

	accessToken := strings.TrimPrefix(authHeader, "Bearer ")
	data := NewAuthService().GenerateRefreshToken(r.Context(), accessToken)
	utils.WriteToJson(w, http.StatusOK, data)
}

func (config *AuthServiceConfig) ValidateEmailHandler(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "invalid request! auth token not found", http.StatusBadRequest)
		return
	}

	data := NewAuthService().ValidateEmail(r.Context(), token)
	utils.WriteToJson(w, http.StatusOK, data)

}
