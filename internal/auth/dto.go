package auth

import "github.com/The-True-Hooha/stellance-backend.git/internal/user"

type AuthRequestDto struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,passwd"`
}

type AuthResponseDto struct {
	User               user.UserProfileDto `json:"user,omitempty"`
	AccessToken        string              `json:"access_token,omitempty"`
	RefreshToken       string              `json:"refresh_token,omitempty"`
	ExpiresIn          int64               `json:"expires_in,omitempty"`
	RefreshTokenExpiry int64               `json:"refresh_token_expiry,omitempty"`
	EmailVerified      bool                `json:"email_verified"`
	ProfileComplete    bool                `json:"profile_complete"`
}
type AuthLoginResponseDto struct {
	AccessToken     string    `json:"access_token,omitempty"`
	ExpiresIn       int64     `json:"expires_in,omitempty"`
	EmailVerified   bool      `json:"email_verified"`
	ProfileComplete bool      `json:"profile_complete"`
	User            user.User `json:"user"`
}

type ResetPasswordDto struct {
	Email           string `json:"email" validate:"required,email"`
	Password        string `json:"password" validate:"required,passwd"`
	ConfirmPassword string `json:"confirm_password" validate:"required,passwd"`
	Otp             string `json:"otp" validate:"required,min=2,max=6"`
}
