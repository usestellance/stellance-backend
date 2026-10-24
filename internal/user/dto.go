package user

import "time"

type UserRole string

const (
	RoleUser  UserRole = "user"
	RoleAdmin UserRole = "admin"
)

type UserProfileDto struct {
	ID              string      `json:"id" example:"3f2b8c1e-5d4a-4f6b-9c7d-1a2b3c4d5e6f"` // User id
	Email           string      `json:"email,omitempty" example:"jane@example.com"`        // Email
	Password        *string     `json:"password,omitempty" swaggerignore:"true"`
	FirstName       *string     `json:"first_name,omitempty" example:"Jane"`                        // First name
	LastName        *string     `json:"last_name,omitempty" example:"Doe"`                          // Last name
	BusinessName    *string     `json:"business_name,omitempty" example:"Doe Studio"`               // Business
	PhoneNumber     *string     `json:"phone_number,omitempty" example:"+2348012345678"`            // Phone
	Country         *string     `json:"country,omitempty" example:"Nigeria"`                        // Country
	IsActive        bool        `json:"is_active" example:"true"`                                   // Active flag
	EmailVerified   bool        `json:"email_verified,omitempty" example:"true"`                    // Email verified
	EmailVerifiedAt *time.Time  `json:"email_verified_at,omitempty" example:"2026-10-20T09:00:00Z"` // When verified
	CreatedAt       *time.Time  `json:"created_at,omitempty" example:"2026-10-20T09:00:00Z"`        // Created
	UpdatedAt       *time.Time  `json:"updated_at,omitempty" example:"2026-10-22T09:00:00Z"`        // Updated
	Role            UserRole    `json:"role,omitempty" example:"user"`                              // user or admin
	Wallet          *UserWallet `json:"wallet,omitempty"`                                           // Primary wallet
	AuthType        string      `json:"auth_type,omitempty" example:"password"`                     // password or google
	ProviderID      *string     `json:"provider_id,omitempty" swaggerignore:"true"`
}

type CompleteProfileRequestDto struct {
	FirstName    string `json:"first_name" validate:"required,min=2,max=15" example:"Jane"`                  // 2-15 characters
	LastName     string `json:"last_name" validate:"required,min=2,max=15" example:"Doe"`                    // 2-15 characters
	BusinessName string `json:"business_name,omitempty" validate:"omitempty,max=50" example:"Doe Studio"`    // Optional, max 50
	PhoneNumber  string `json:"phone_number,omitempty" validate:"omitempty,max=15" example:"+2348012345678"` // Optional, max 15
	Country      string `json:"country,omitempty" validate:"omitempty,max=25" example:"Nigeria"`             // Optional, max 25
}

type UpdateProfileDto struct {
	FirstName    *string `json:"first_name,omitempty" validate:"omitempty,min=2,max=15" example:"Jane"`       // 2-15 characters
	LastName     *string `json:"last_name,omitempty" validate:"omitempty,min=2,max=15" example:"Doe"`         // 2-15 characters
	BusinessName *string `json:"business_name,omitempty" validate:"omitempty,max=50" example:"Doe Studio"`    // Max 50
	PhoneNumber  *string `json:"phone_number,omitempty" validate:"omitempty,max=15" example:"+2348012345678"` // Max 15
	Country      *string `json:"country,omitempty" validate:"omitempty,max=25" example:"Nigeria"`             // Max 25
}

type User struct {
	Id            string      `json:"id" example:"3f2b8c1e-5d4a-4f6b-9c7d-1a2b3c4d5e6f"` // User id
	FirstName     *string     `json:"first_name,omitempty" example:"Jane"`               // First name
	LastName      *string     `json:"last_name,omitempty" example:"Doe"`                 // Last name
	Email         string      `json:"email" example:"jane@example.com"`                  // Email
	BusinessName  *string     `json:"business_name,omitempty" example:"Doe Studio"`      // Business
	PhoneNumber   *string     `json:"phone_number,omitempty" example:"+2348012345678"`   // Phone
	Country       *string     `json:"country,omitempty" example:"Nigeria"`               // Country
	EmailVerified *bool       `json:"email_verified,omitempty" example:"true"`           // Email verified
	Wallet        *UserWallet `json:"wallet,omitempty"`
}

type UserWallet struct {
	Id      *string        `json:"id,omitempty" example:"3f2b8c1e-5d4a-4f6b-9c7d-1a2b3c4d5e6f"`                          // Wallet id, needed by PIN, transfer and export endpoints
	Address *string        `json:"address,omitempty" example:"GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"` // Stellar account id
	Balance *WalletBalance `json:"balance,omitempty"`                                                                    // USDC and XLM balances
}

type WalletBalance struct {
	USDC *float64 `json:"usdc,omitempty" example:"5.03"` // USDC
	XLM  *float64 `json:"xlm,omitempty" example:"10002"` // XLM
}
