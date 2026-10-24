package auth

// RefreshTokenResult is the data of POST /auth/token.
type RefreshTokenResult struct {
	RefreshToken       string `json:"refresh_token" example:"eyJhbGciOi..."` // 7-day refresh token
	RefreshTokenExpiry int64  `json:"refresh_token_expiry" example:"1793000000"`
}

// PasswordChangedResult is the data of POST /auth/change-password.
type PasswordChangedResult struct {
	ChangedAt string `json:"changedAt" example:"2026-10-22T14:03:26Z"` // When the password changed
}

// SocialSignupResult is the data returned when POST /auth/social creates a new account.
type SocialSignupResult struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in" example:"1793000000"`
}

// EmailNotVerifiedResult is the data of the 403 returned by POST /auth/login for unverified accounts.
type EmailNotVerifiedResult struct {
	EmailVerified bool `json:"email_verified" example:"false"`
}
