package auth

import (
	"github.com/The-True-Hooha/stellance-backend.git/internal/user"
)

type AuthRequestDto struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8,containsany=!@#$%^&*"`
}

type AuthResponseDto struct {
	User               user.UserResponseDto `json:"user,omitempty"`
	AccessToken        string               `json:"access_token,omitempty"`
	RefreshToken       string               `json:"refresh_token,omitempty"`
	ExpiresIn          int64                `json:"expires_in,omitempty"`
	RefreshTokenExpiry int64                `json:"refresh_token_expiry,omitempty"`
}
