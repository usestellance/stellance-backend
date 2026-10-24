package transactions

// PaymentStatusResult is the data of GET /transaction/status/{id}.
type PaymentStatusResult struct {
	TransactionID   string `json:"transaction_id" example:"3f2b8c1e-5d4a-4f6b-9c7d-1a2b3c4d5e6f"`                               // Ledger entry id
	TransactionHash string `json:"transaction_hash" example:"a9f9fde3c0e1d5b7a9f9fde3c0e1d5b7a9f9fde3c0e1d5b7a9f9fde3c0e1d5b7"` // Stellar transaction hash
	Status          string `json:"status" enums:"pending,confirmed,failed,aborted,refunded"`
	StellarURL      string `json:"stellar_url" example:"https://stellar.expert/explorer/testnet/tx/a9f9fde3c0e1d5b7a9f9fde3c0e1d5b7a9f9fde3c0e1d5b7a9f9fde3c0e1d5b7"` // Explorer link
}
