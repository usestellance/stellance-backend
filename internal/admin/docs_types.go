package admin

// AdminUserList is the data of GET /admin/users.
type AdminUserList struct {
	Users []AdminUserRow      `json:"users"`
	Meta  AdminPaginationMeta `json:"meta"`
}

// AdminUserDetail is the data of GET /admin/users/{id}. Empty collections are [], never null.
type AdminUserDetail struct {
	User               AdminUserRow          `json:"user"`
	Wallets            []AdminWalletRow      `json:"wallets"`
	RecentTransactions []AdminTransactionRow `json:"recent_transactions"`
	RecentActivity     []ActivityLog         `json:"recent_activity"`
}

// AdminInvoiceList is the data of GET /admin/invoices and GET /admin/users/{id}/invoices.
type AdminInvoiceList struct {
	Invoices []AdminInvoiceRow   `json:"invoices"`
	Meta     AdminPaginationMeta `json:"meta"`
}

// AdminTransactionList is the data of GET /admin/transactions and GET /admin/users/{id}/transactions.
type AdminTransactionList struct {
	Transactions []AdminTransactionRow `json:"transactions"`
	Meta         AdminPaginationMeta   `json:"meta"`
}

// AdminActivityList is the data of GET /admin/users/{id}/activity.
type AdminActivityList struct {
	Logs []ActivityLog       `json:"logs"`
	Meta AdminPaginationMeta `json:"meta"`
}

// NetworkConfig is the data of GET /admin/config/network.
type NetworkConfig struct {
	Stage     string `json:"stage" enums:"testnet,mainnet"`
	UpdatedAt string `json:"updated_at" example:"2026-10-22T14:03:26Z"` // When the setting last changed
}

// NetworkUpdate is the body of PATCH /admin/config/network.
type NetworkUpdate struct {
	Stage string `json:"stage" enums:"testnet,mainnet" example:"testnet"` // Network used by wallets and payment detection
}
