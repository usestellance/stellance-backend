package user

import "time"

type UserRole string

const (
	RoleUser  UserRole = "user"
	RoleAdmin UserRole = "admin"
)

type UserProfileDto struct {
	ID              string     `json:"id"`
	Email           string     `json:"email,omitempty"`
	Password        string     `json:"password,omitempty"`
	FirstName       *string    `json:"first_name,omitempty"`
	LastName        *string    `json:"last_name,omitempty"`
	BusinessName    *string    `json:"business_name,omitempty"`
	PhoneNumber     *string    `json:"phone_number,omitempty"`
	Country         string    `json:"country,omitempty"`
	IsActive        bool       `json:"is_active"`
	EmailVerified   bool       `json:"email_verified,omitempty"`
	EmailVerifiedAt *time.Time `json:"email_verified_at,omitempty"`
	CreatedAt       *time.Time `json:"created_at,omitempty"`
	UpdatedAt       *time.Time `json:"updated_at,omitempty"`
	Role            UserRole   `json:"role,omitempty"`
}

type CompleteProfileRequestDto struct {
	FirstName    string `json:"first_name" validate:"required,min=2,max=15"`
	LastName     string `json:"last_name" validate:"required,min=2,max=15"`
	BusinessName string `json:"business_name,omitempty" validate:"omitempty,max=50"`
	PhoneNumber  string `json:"phone_number,omitempty" validate:"omitempty,max=15"`
	Country      string `json:"country,omitempty" validate:"omitempty,max=25"`
}

type UpdateProfileDto struct {
	FirstName    *string `json:"first_name,omitempty" validate:"omitempty,min=2,max=15"`
	LastName     *string `json:"last_name,omitempty" validate:"omitempty,min=2,max=15"`
	BusinessName *string `json:"business_name,omitempty" validate:"omitempty,max=50"`
	PhoneNumber  *string `json:"phone_number,omitempty" validate:"omitempty,max=15"`
	Country      *string `json:"country,omitempty" validate:"omitempty,max=25"`
}

type User struct {
	Id           string     `json:"id"`
	Name         string     `json:"name"`
	Email        string     `json:"email"`
	BusinessName *string    `json:"business_name,omitempty"`
	PhoneNumber  *string    `json:"phone_number,omitempty"`
	Country      string     `json:"country"`
	Wallet       *UserWallet `json:"wallet,omitempty"`
}

type UserWallet struct {
	Address string  `json:"address,omitempty"`
	Balance float64 `json:"balance,omitempty"`
}
