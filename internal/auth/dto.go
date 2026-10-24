package auth

import "github.com/The-True-Hooha/stellance-backend/internal/user"

type AuthRequestDto struct {
	Email    string `json:"email" validate:"required,email" example:"jane@example.com"` // Account email, case-insensitive
	Password string `json:"password" validate:"required,passwd" example:"S3cure!pass"`  // Password; must satisfy the password policy
}

type AuthResponseDto struct {
	User               user.UserProfileDto `json:"user,omitempty"`
	AccessToken        string              `json:"access_token,omitempty" example:"eyJhbGciOi..."`      // Bearer token, valid for 1 hour
	RefreshToken       string              `json:"refresh_token,omitempty"`                             // Only on the refresh endpoint
	ExpiresIn          int64               `json:"expires_in,omitempty" example:"1793000000"`           // Unix time the access token expires
	RefreshTokenExpiry int64               `json:"refresh_token_expiry,omitempty" example:"1793600000"` // Unix time the refresh token expires
	EmailVerified      bool                `json:"email_verified" example:"false"`                      // New accounts start unverified
	ProfileComplete    bool                `json:"profile_complete" example:"false"`                    // Profile onboarding done
}
type AuthLoginResponseDto struct {
	AccessToken     string     `json:"access_token,omitempty" example:"eyJhbGciOi..."` // Bearer token, valid for 1 hour
	ExpiresIn       int64      `json:"expires_in,omitempty" example:"1793000000"`      // Unix time the token expires
	EmailVerified   bool       `json:"email_verified" example:"true"`                  // Always true on a successful login
	ProfileComplete bool       `json:"profile_complete" example:"false"`               // False until POST /profile has been called
	User            *user.User `json:"user,omitempty"`
}

type ResetPasswordDto struct {
	Email           string `json:"email" validate:"required,email" example:"jane@example.com"`         // Email the reset link was sent to
	Password        string `json:"password" validate:"required,passwd" example:"N3w!password"`         // New password
	ConfirmPassword string `json:"confirm_password" validate:"required,passwd" example:"N3w!password"` // Must equal password
	Otp             string `json:"otp" validate:"required,min=2,max=6" example:"48213"`                // Code from the reset email
}

type ChangePasswordDTO struct {
	OldPassword string `json:"old_password" validate:"required,min=8.passwd" example:"S3cure!pass"`          // Current password
	NewPassword string `json:"new_password" validate:"required,min=8,max=100,passwd" example:"N3w!password"` // New password, 8-100 characters
}

type ProviderLogin struct {
	IDToken    string `json:"id_token" validate:"required" example:"eyJhbGciOiJSUzI1NiIs..."` // Google ID token (JWT) from Google Sign-In; verified server-side
	Email      string `json:"-"`
	ProviderID string `json:"-"`
}
