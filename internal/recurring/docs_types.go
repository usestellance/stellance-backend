package recurring

// RecurringCreated is the data of POST /recurring.
type RecurringCreated struct {
	ID        string `json:"id" example:"3f2b8c1e-5d4a-4f6b-9c7d-1a2b3c4d5e6f"` // Schedule id
	NextRunAt string `json:"next_run_at" example:"2026-11-01T00:00:00Z"`        // First run
}
