package user

// ProfileResult is the data of GET /profile/{id} and GET /profile/me.
type ProfileResult struct {
	Profile         UserProfileDto `json:"profile"`
	ProfileComplete bool           `json:"profile_complete"`
	Wallet          UserWallet     `json:"wallet"`
}

// CompleteProfileResult is the data of POST /profile.
type CompleteProfileResult struct {
	ID               string  `json:"id" example:"3f2b8c1e-5d4a-4f6b-9c7d-1a2b3c4d5e6f"` // User id
	Email            string  `json:"email" example:"jane@example.com"`                  // Email
	FirstName        *string `json:"first_name" example:"Jane"`                         // First name
	LastName         *string `json:"last_name" example:"Doe"`                           // Last name
	BusinessName     *string `json:"business_name" example:"Doe Studio"`                // Business
	PhoneNumber      *string `json:"phone_number" example:"+2348012345678"`             // Phone
	Country          *string `json:"country" example:"Nigeria"`                         // Country
	ProfileCompleted bool    `json:"profile_completed" example:"true"`
}
