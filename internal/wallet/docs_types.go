package wallet

// WalletExportResult is the data of GET /wallet/export/{id}. It contains the secret key: never log or cache it.
type WalletExportResult struct {
	PrivateKey    string `json:"private_key" example:"S..."`                                                        // Secret key. Never log or cache
	WalletAddress string `json:"wallet_address" example:"GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"` // Stellar account id
}

// WalletLookupResult is the data of GET /wallet/lookup.
type WalletLookupResult struct {
	WalletAddress string `json:"wallet_address" example:"GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"` // Recipient account
	UserID        string `json:"user_id" example:"3f2b8c1e-5d4a-4f6b-9c7d-1a2b3c4d5e6f"`                            // Wallet owner
}
