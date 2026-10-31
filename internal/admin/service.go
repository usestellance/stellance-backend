package admin

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/big"
	"net/http"
	"net/url"
	"time"

	"github.com/The-True-Hooha/stellance-backend/internal/activitylog"
	"github.com/The-True-Hooha/stellance-backend/internal/balance"
	"github.com/The-True-Hooha/stellance-backend/mail"
	"github.com/The-True-Hooha/stellance-backend/pkg/config"
	jwt_ "github.com/The-True-Hooha/stellance-backend/pkg/jwt"
	"github.com/The-True-Hooha/stellance-backend/pkg/utils"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type AdminService struct {
	log      *slog.Logger
	postgres *pgxpool.Pool
	redis    *redis.Client
	jwt      *jwt_.JwtTokenServiceConfig
}

func NewAdminService() *AdminService {
	return &AdminService{
		log:      config.GetAppContainer().Log,
		postgres: config.GetAppContainer().Postgres,
		redis:    config.GetAppContainer().Redis,
		jwt:      jwt_.JwtTokenService(),
	}
}

type AdminUserRow struct {
	ID            string    `json:"id" example:"3f2b8c1e-5d4a-4f6b-9c7d-1a2b3c4d5e6f"` // User id
	Email         string    `json:"email" example:"jane@example.com"`                  // Email
	FirstName     string    `json:"first_name" example:"Jane"`                         // First name
	LastName      string    `json:"last_name" example:"Doe"`                           // Last name
	Role          string    `json:"role" example:"user"`                               // user or admin
	EmailVerified bool      `json:"email_verified" example:"true"`                     // Verified
	IsActive      bool      `json:"is_active" example:"true"`                          // Active
	CreatedAt     time.Time `json:"created_at" example:"2026-10-20T09:00:00Z"`         // Created
}

type AdminInvoiceRow struct {
	ID            string    `json:"id" example:"3f2b8c1e-5d4a-4f6b-9c7d-1a2b3c4d5e6f"` // Invoice id
	InvoiceNumber string    `json:"invoice_number" example:"INV-20261020-0019"`        // Number
	CreatorEmail  string    `json:"creator_email" example:"jane@example.com"`          // Creator
	PayerEmail    string    `json:"payer_email" example:"billing@acme.com"`            // Client
	Total         float64   `json:"total" example:"454"`                               // Amount due
	Currency      string    `json:"currency" example:"usdc"`                           // Currency
	Status        string    `json:"status" example:"paid"`                             // Invoice status
	CreatedAt     time.Time `json:"created_at" example:"2026-10-20T09:00:00Z"`         // Created
}

type AdminStats struct {
	TotalUsers    int     `json:"total_users" example:"120"`       // All users
	ActiveUsers   int     `json:"active_users" example:"112"`      // Active users
	TotalInvoices int     `json:"total_invoices" example:"560"`    // All invoices
	PaidInvoices  int     `json:"paid_invoices" example:"310"`     // Paid invoices
	OverdueCount  int     `json:"overdue_count" example:"12"`      // Invoices in overdue status
	TotalRevenue  float64 `json:"total_revenue" example:"48210.5"` // Sum of paid invoice totals
}

type AdminWalletRow struct {
	ID           string   `json:"id" example:"3f2b8c1e-5d4a-4f6b-9c7d-1a2b3c4d5e6f"`                          // Wallet id
	Address      string   `json:"address" example:"GBBD47IF6LWK7P7MDEVSCWR7DPUWV3NY3DTQEVFL4NAT4AQH3ZLLFLA5"` // Stellar account id
	Tag          string   `json:"tag" example:"Main"`                                                         // Label
	USDC         *float64 `json:"usdc_balance" example:"5.03"`                                                // Live USDC balance from Horizon; null when it could not be read
	XLM          *float64 `json:"xlm_balance" example:"10002"`                                                // Live XLM balance from Horizon; null when it could not be read
	BalanceError string   `json:"balance_error,omitempty" example:"could not read the balance from Stellar"`  // Set when the live balance could not be read
	Primary      bool     `json:"is_primary" example:"true"`                                                  // Receives invoice payments
}

type AdminPaginationMeta struct {
	Page       int `json:"page" example:"1"`        // Current page
	Limit      int `json:"limit" example:"20"`      // Page size
	Total      int `json:"total" example:"57"`      // Total matching rows
	TotalPages int `json:"total_pages" example:"3"` // Total pages
}

func (as *AdminService) GetStats(ctx context.Context) *utils.ApiResponse {
	var stats AdminStats
	err := as.postgres.QueryRow(ctx, `
		SELECT
			COUNT(*) AS total_users,
			COUNT(*) FILTER (WHERE is_active = TRUE) AS active_users
		FROM users
	`).Scan(&stats.TotalUsers, &stats.ActiveUsers)
	if err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusInternalServerError, Message: "failed to fetch user stats"}
	}

	err = as.postgres.QueryRow(ctx, `
		SELECT
			COUNT(*) AS total_invoices,
			COUNT(*) FILTER (WHERE status = 'paid') AS paid_invoices,
			COUNT(*) FILTER (WHERE status = 'overdue') AS overdue_count,
			COALESCE(SUM(total) FILTER (WHERE status = 'paid'), 0) AS total_revenue
		FROM invoice
	`).Scan(&stats.TotalInvoices, &stats.PaidInvoices, &stats.OverdueCount, &stats.TotalRevenue)
	if err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusInternalServerError, Message: "failed to fetch invoice stats"}
	}

	return &utils.ApiResponse{StatusCode: http.StatusOK, Message: "successful", Data: stats}
}

func (as *AdminService) ListUsers(ctx context.Context, page, limit int, search string) *utils.ApiResponse {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	query := `SELECT id, email, COALESCE(first_name,''), COALESCE(last_name,''), permission::text, email_verified, is_active, created_at FROM users`
	countQuery := `SELECT COUNT(*) FROM users`
	args := []any{}
	argCount := 0

	if search != "" {
		argCount++
		query += fmt.Sprintf(" WHERE email ILIKE $%d OR first_name ILIKE $%d OR last_name ILIKE $%d", argCount, argCount, argCount)
		countQuery += fmt.Sprintf(" WHERE email ILIKE $%d OR first_name ILIKE $%d OR last_name ILIKE $%d", argCount, argCount, argCount)
		args = append(args, "%"+search+"%")
	}

	var total int
	if err := as.postgres.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusInternalServerError, Message: "failed to count users"}
	}

	argCount++
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", argCount)
	args = append(args, limit)
	argCount++
	query += fmt.Sprintf(" OFFSET $%d", argCount)
	args = append(args, offset)

	rows, err := as.postgres.Query(ctx, query, args...)
	if err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusInternalServerError, Message: "failed to list users"}
	}
	defer rows.Close()

	users := []AdminUserRow{}
	for rows.Next() {
		var u AdminUserRow
		if err := rows.Scan(&u.ID, &u.Email, &u.FirstName, &u.LastName, &u.Role, &u.EmailVerified, &u.IsActive, &u.CreatedAt); err != nil {
			continue
		}
		users = append(users, u)
	}

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "successful",
		Data: map[string]any{
			"users": users,
			"meta": AdminPaginationMeta{
				Page:       page,
				Limit:      limit,
				Total:      total,
				TotalPages: int(math.Ceil(float64(total) / float64(limit))),
			},
		},
	}
}

func (as *AdminService) GetUser(ctx context.Context, userID string) *utils.ApiResponse {
	var u AdminUserRow
	err := as.postgres.QueryRow(ctx,
		`SELECT id, email, COALESCE(first_name,''), COALESCE(last_name,''), permission::text, email_verified, is_active, created_at
		 FROM users WHERE id = $1`, userID,
	).Scan(&u.ID, &u.Email, &u.FirstName, &u.LastName, &u.Role, &u.EmailVerified, &u.IsActive, &u.CreatedAt)
	if err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusNotFound, Message: "user not found"}
	}

	txRows, _ := as.postgres.Query(ctx,
		`SELECT t.id, COALESCE(i.invoice_number,''), u.email, t.amount, t.transaction_type::text, t.status::text, t.source_asset, t.source_amount, t.created_at
		 FROM transactions t
		 JOIN users u ON u.id = t.user_id
		 LEFT JOIN invoice i ON i.id = t.invoice_id
		 WHERE t.user_id = $1 ORDER BY t.created_at DESC LIMIT 10`, userID,
	)
	recentTx := []AdminTransactionRow{}
	if txRows != nil {
		defer txRows.Close()
		for txRows.Next() {
			var tx AdminTransactionRow
			if err := txRows.Scan(&tx.ID, &tx.InvoiceNumber, &tx.UserEmail, &tx.Amount, &tx.Type, &tx.Status, &tx.SourceAsset, &tx.SourceAmount, &tx.CreatedAt); err == nil {
				recentTx = append(recentTx, tx)
			}
		}
	}

	logRows, _ := as.postgres.Query(ctx,
		`SELECT id, user_id, action, COALESCE(entity_type,''), COALESCE(entity_id::text,''), COALESCE(ip_address,''), created_at
		 FROM user_activity_logs WHERE user_id = $1 ORDER BY created_at DESC LIMIT 20`, userID,
	)
	recentActivity := []ActivityLog{}
	if logRows != nil {
		defer logRows.Close()
		for logRows.Next() {
			var l ActivityLog
			if err := logRows.Scan(&l.ID, &l.UserID, &l.Action, &l.EntityType, &l.EntityID, &l.IPAddress, &l.CreatedAt); err == nil {
				recentActivity = append(recentActivity, l)
			}
		}
	}

	wRows, _ := as.postgres.Query(ctx,
		`SELECT id, address, COALESCE(tag,''), is_primary
		 FROM wallets WHERE user_id = $1 ORDER BY is_primary DESC`, userID,
	)
	wallets := []AdminWalletRow{}
	if wRows != nil {
		defer wRows.Close()
		for wRows.Next() {
			var w AdminWalletRow
			if err := wRows.Scan(&w.ID, &w.Address, &w.Tag, &w.Primary); err != nil {
				continue
			}
			if b, berr := balance.Fetch(ctx, w.Address); berr != nil {
				as.log.Error("failed to read wallet balance from horizon", "error", berr, "address", w.Address)
				w.BalanceError = "could not read the balance from Stellar"
			} else {
				w.USDC, w.XLM = &b.USDC, &b.XLM
			}
			wallets = append(wallets, w)
		}
	}

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "successful",
		Data: map[string]any{
			"user":                u,
			"wallets":             wallets,
			"recent_transactions": recentTx,
			"recent_activity":     recentActivity,
		},
	}
}

func (as *AdminService) SetUserActive(ctx context.Context, userID string, active bool, adminID, ip string) *utils.ApiResponse {
	tag, err := as.postgres.Exec(ctx, `UPDATE users SET is_active = $1, updated_at = NOW() WHERE id = $2`, active, userID)
	if err != nil || tag.RowsAffected() == 0 {
		return &utils.ApiResponse{StatusCode: http.StatusNotFound, Message: "user not found"}
	}
	action := activitylog.ActionAdminActivate
	msg := "user activated"
	if !active {
		action = activitylog.ActionAdminDeactivate
		msg = "user deactivated"
	}
	as.log_(ctx, adminID, action, activitylog.EntityUser, userID, ip)
	return &utils.ApiResponse{StatusCode: http.StatusOK, Message: msg}
}

func (as *AdminService) DeleteUser(ctx context.Context, userID string, adminID, ip string) *utils.ApiResponse {
	tag, err := as.postgres.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		return &utils.ApiResponse{StatusCode: http.StatusConflict, Message: "user owns invoices and cannot be deleted"}
	}
	if err != nil || tag.RowsAffected() == 0 {
		return &utils.ApiResponse{StatusCode: http.StatusNotFound, Message: "user not found"}
	}
	as.log_(ctx, adminID, activitylog.ActionAdminDelete, activitylog.EntityUser, userID, ip)
	return &utils.ApiResponse{StatusCode: http.StatusOK, Message: "user deleted"}
}

func (as *AdminService) GetInvoice(ctx context.Context, invoiceID string) *utils.ApiResponse {
	var inv AdminInvoiceRow
	err := as.postgres.QueryRow(ctx,
		`SELECT i.id, i.invoice_number, u.email, i.payer_email, i.total, i.currency::text, i.status::text, i.created_at
		 FROM invoice i JOIN users u ON u.id = i.created_by_id WHERE i.id = $1`, invoiceID,
	).Scan(&inv.ID, &inv.InvoiceNumber, &inv.CreatorEmail, &inv.PayerEmail, &inv.Total, &inv.Currency, &inv.Status, &inv.CreatedAt)
	if err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusNotFound, Message: "invoice not found"}
	}
	return &utils.ApiResponse{StatusCode: http.StatusOK, Message: "successful", Data: inv}
}

type AdminTransactionRow struct {
	ID            string    `json:"id" example:"3f2b8c1e-5d4a-4f6b-9c7d-1a2b3c4d5e6f"` // Ledger entry id
	InvoiceNumber string    `json:"invoice_number" example:"INV-20261020-0019"`        // Related invoice
	UserEmail     string    `json:"user_email" example:"jane@example.com"`             // Owner
	Amount        float64   `json:"amount" example:"2.03"`                             // Amount
	Type          string    `json:"type" example:"payment"`                            // payment, withdrawal, funding or path_payment
	Status        string    `json:"status" example:"confirmed"`                        // On-chain result
	SourceAsset   *string   `json:"source_asset,omitempty" example:"XLM"`              // Asset spent for path payments
	SourceAmount  *string   `json:"source_amount,omitempty" example:"12.3456789"`      // Amount spent
	CreatedAt     time.Time `json:"created_at" example:"2026-10-22T14:03:26Z"`         // Created
}

func (as *AdminService) ListTransactions(ctx context.Context, page, limit int, search string) *utils.ApiResponse {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	base := `SELECT t.id, COALESCE(i.invoice_number,''), u.email, t.amount, t.transaction_type::text, t.status::text, t.source_asset, t.source_amount, t.created_at
			 FROM transactions t
			 JOIN users u ON u.id = t.user_id
			 LEFT JOIN invoice i ON i.id = t.invoice_id`
	countBase := `SELECT COUNT(*) FROM transactions t JOIN users u ON u.id = t.user_id LEFT JOIN invoice i ON i.id = t.invoice_id`

	args := []any{}
	where := ""
	if search != "" {
		where = ` WHERE (u.email ILIKE $1 OR i.invoice_number ILIKE $1)`
		args = append(args, "%"+search+"%")
	}

	var total int
	if err := as.postgres.QueryRow(ctx, countBase+where, args...).Scan(&total); err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusInternalServerError, Message: "failed to count transactions"}
	}

	limitArg := len(args) + 1
	offsetArg := len(args) + 2
	query := fmt.Sprintf("%s%s ORDER BY t.created_at DESC LIMIT $%d OFFSET $%d", base, where, limitArg, offsetArg)
	args = append(args, limit, offset)

	rows, err := as.postgres.Query(ctx, query, args...)
	if err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusInternalServerError, Message: "failed to list transactions"}
	}
	defer rows.Close()

	txs := []AdminTransactionRow{}
	for rows.Next() {
		var tx AdminTransactionRow
		if err := rows.Scan(&tx.ID, &tx.InvoiceNumber, &tx.UserEmail, &tx.Amount, &tx.Type, &tx.Status, &tx.SourceAsset, &tx.SourceAmount, &tx.CreatedAt); err != nil {
			continue
		}
		txs = append(txs, tx)
	}

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "successful",
		Data: map[string]any{
			"transactions": txs,
			"meta": AdminPaginationMeta{
				Page:       page,
				Limit:      limit,
				Total:      total,
				TotalPages: int(math.Ceil(float64(total) / float64(limit))),
			},
		},
	}
}

type ActivityLog struct {
	ID         string    `json:"id" example:"3f2b8c1e-5d4a-4f6b-9c7d-1a2b3c4d5e6f"`                  // Entry id
	UserID     string    `json:"user_id" example:"3f2b8c1e-5d4a-4f6b-9c7d-1a2b3c4d5e6f"`             // Acting user
	Action     string    `json:"action" example:"login"`                                             // Action name
	EntityType string    `json:"entity_type,omitempty" example:"invoice"`                            // Kind of object acted on
	EntityID   string    `json:"entity_id,omitempty" example:"3f2b8c1e-5d4a-4f6b-9c7d-1a2b3c4d5e6f"` // Id of the object acted on
	Metadata   any       `json:"metadata,omitempty"`
	IPAddress  string    `json:"ip_address,omitempty" example:"203.0.113.7"` // Client IP
	CreatedAt  time.Time `json:"created_at" example:"2026-10-22T14:03:26Z"`  // When it happened
}

func (as *AdminService) log_(ctx context.Context, userID, action, entityType, entityID, ip string) {
	activitylog.Log(ctx, as.postgres, as.log, userID, action, entityType, entityID, ip)
}

func (as *AdminService) GetUserInvoices(ctx context.Context, userID string, page, limit int) *utils.ApiResponse {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	var total int
	if err := as.postgres.QueryRow(ctx, `SELECT COUNT(*) FROM invoice WHERE created_by_id = $1`, userID).Scan(&total); err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusInternalServerError, Message: "failed to count invoices"}
	}

	rows, err := as.postgres.Query(ctx,
		`SELECT i.id, i.invoice_number, u.email, i.payer_email, i.total, i.currency::text, i.status::text, i.created_at
		 FROM invoice i JOIN users u ON u.id = i.created_by_id
		 WHERE i.created_by_id = $1
		 ORDER BY i.created_at DESC LIMIT $2 OFFSET $3`,
		userID, limit, offset,
	)
	if err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusInternalServerError, Message: "failed to fetch invoices"}
	}
	defer rows.Close()

	invoices := []AdminInvoiceRow{}
	for rows.Next() {
		var inv AdminInvoiceRow
		if err := rows.Scan(&inv.ID, &inv.InvoiceNumber, &inv.CreatorEmail, &inv.PayerEmail, &inv.Total, &inv.Currency, &inv.Status, &inv.CreatedAt); err != nil {
			continue
		}
		invoices = append(invoices, inv)
	}

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "successful",
		Data: map[string]any{
			"invoices": invoices,
			"meta": AdminPaginationMeta{
				Page:       page,
				Limit:      limit,
				Total:      total,
				TotalPages: int(math.Ceil(float64(total) / float64(limit))),
			},
		},
	}
}

func (as *AdminService) GetUserTransactions(ctx context.Context, userID string, page, limit int) *utils.ApiResponse {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	var total int
	if err := as.postgres.QueryRow(ctx, `SELECT COUNT(*) FROM transactions WHERE user_id = $1`, userID).Scan(&total); err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusInternalServerError, Message: "failed to count transactions"}
	}

	rows, err := as.postgres.Query(ctx,
		`SELECT t.id, COALESCE(i.invoice_number,''), u.email, t.amount, t.transaction_type::text, t.status::text, t.source_asset, t.source_amount, t.created_at
		 FROM transactions t
		 JOIN users u ON u.id = t.user_id
		 LEFT JOIN invoice i ON i.id = t.invoice_id
		 WHERE t.user_id = $1
		 ORDER BY t.created_at DESC LIMIT $2 OFFSET $3`,
		userID, limit, offset,
	)
	if err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusInternalServerError, Message: "failed to fetch transactions"}
	}
	defer rows.Close()

	txs := []AdminTransactionRow{}
	for rows.Next() {
		var tx AdminTransactionRow
		if err := rows.Scan(&tx.ID, &tx.InvoiceNumber, &tx.UserEmail, &tx.Amount, &tx.Type, &tx.Status, &tx.SourceAsset, &tx.SourceAmount, &tx.CreatedAt); err != nil {
			continue
		}
		txs = append(txs, tx)
	}

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "successful",
		Data: map[string]any{
			"transactions": txs,
			"meta": AdminPaginationMeta{
				Page:       page,
				Limit:      limit,
				Total:      total,
				TotalPages: int(math.Ceil(float64(total) / float64(limit))),
			},
		},
	}
}

func (as *AdminService) GetUserActivity(ctx context.Context, userID string, page, limit int) *utils.ApiResponse {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	var total int
	if err := as.postgres.QueryRow(ctx, `SELECT COUNT(*) FROM user_activity_logs WHERE user_id = $1`, userID).Scan(&total); err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusInternalServerError, Message: "failed to count activity logs"}
	}

	rows, err := as.postgres.Query(ctx,
		`SELECT id, user_id, action, COALESCE(entity_type,''), COALESCE(entity_id::text,''), COALESCE(ip_address,''), created_at
		 FROM user_activity_logs WHERE user_id = $1
		 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		userID, limit, offset,
	)
	if err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusInternalServerError, Message: "failed to fetch activity logs"}
	}
	defer rows.Close()

	logs := []ActivityLog{}
	for rows.Next() {
		var l ActivityLog
		if err := rows.Scan(&l.ID, &l.UserID, &l.Action, &l.EntityType, &l.EntityID, &l.IPAddress, &l.CreatedAt); err != nil {
			continue
		}
		logs = append(logs, l)
	}

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "successful",
		Data: map[string]any{
			"logs": logs,
			"meta": AdminPaginationMeta{
				Page:       page,
				Limit:      limit,
				Total:      total,
				TotalPages: int(math.Ceil(float64(total) / float64(limit))),
			},
		},
	}
}

const resetOTPCacheDuration = 10 * time.Minute

func (as *AdminService) AdminResetUserPassword(ctx context.Context, userID, adminID, ip string) *utils.ApiResponse {
	var email, firstName string
	if err := as.postgres.QueryRow(ctx,
		`SELECT email, COALESCE(first_name,'') FROM users WHERE id = $1`, userID,
	).Scan(&email, &firstName); err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusNotFound, Message: "user not found"}
	}

	otp := generateOTP()
	cacheKey := fmt.Sprintf("email_otp_%s", email)
	cacheVal := fmt.Sprintf(`{"email":%q,"otp":%q}`, email, otp)
	if err := as.redis.Set(ctx, cacheKey, cacheVal, resetOTPCacheDuration).Err(); err != nil {
		as.log.Error("failed to store reset otp in redis", "error", err)
		return &utils.ApiResponse{StatusCode: http.StatusInternalServerError, Message: "service unavailable"}
	}

	go func() {
		resetURL := fmt.Sprintf("https://usestellance.com/auth/reset-password?email=%s", url.QueryEscape(email))
		if err := mail.NewMailer().SendResetEmail(email, resetURL, otp); err != nil {
			as.log.Warn("failed to send admin-triggered reset email", "error", err)
		}
	}()

	as.log_(ctx, adminID, activitylog.ActionPasswordReset, activitylog.EntityUser, userID, ip)
	return &utils.ApiResponse{StatusCode: http.StatusOK, Message: "password reset email sent"}
}

func generateOTP() string {
	otp := ""
	for i := 0; i < 6; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			panic("failed to generate secure random OTP digit")
		}
		otp += n.String()
	}
	return otp
}

func (as *AdminService) SetStellarNetwork(ctx context.Context, stage, adminID string) *utils.ApiResponse {
	if stage != "testnet" && stage != "mainnet" {
		return &utils.ApiResponse{StatusCode: http.StatusBadRequest, Message: "stage must be testnet or mainnet"}
	}
	encrypted, err := utils.EncryptValue(stage)
	if err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusInternalServerError, Message: "failed to encrypt config value"}
	}
	_, err = as.postgres.Exec(ctx,
		`INSERT INTO system_config (key, value, updated_at, updated_by)
		 VALUES ('stellar_network', $1, NOW(), $2)
		 ON CONFLICT (key) DO UPDATE SET value = $1, updated_at = NOW(), updated_by = $2`,
		encrypted, adminID,
	)
	if err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusInternalServerError, Message: "failed to update network config"}
	}
	return &utils.ApiResponse{StatusCode: http.StatusOK, Message: "stellar network updated to " + stage}
}

func (as *AdminService) GetStellarNetwork(ctx context.Context) *utils.ApiResponse {
	var encrypted string
	var updatedAt time.Time
	err := as.postgres.QueryRow(ctx,
		`SELECT value, updated_at FROM system_config WHERE key = 'stellar_network'`,
	).Scan(&encrypted, &updatedAt)
	if err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusNotFound, Message: "network config not set"}
	}
	stage, err := utils.DecryptValue(encrypted)
	if err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusInternalServerError, Message: "failed to read network config"}
	}
	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "successful",
		Data:       map[string]any{"stage": stage, "updated_at": updatedAt},
	}
}

func (as *AdminService) ListInvoices(ctx context.Context, page, limit int, status, search string) *utils.ApiResponse {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	baseQuery := `
		SELECT i.id, i.invoice_number, u.email, i.payer_email, i.total, i.currency::text, i.status::text, i.created_at
		FROM invoice i JOIN users u ON u.id = i.created_by_id
	`
	countQuery := `SELECT COUNT(*) FROM invoice i JOIN users u ON u.id = i.created_by_id`
	args := []any{}
	argCount := 0
	where := ""

	if status != "" {
		argCount++
		where += fmt.Sprintf(" WHERE i.status = $%d", argCount)
		args = append(args, status)
	}
	if search != "" {
		argCount++
		if where == "" {
			where += fmt.Sprintf(" WHERE (i.invoice_number ILIKE $%d OR i.payer_email ILIKE $%d OR u.email ILIKE $%d)", argCount, argCount, argCount)
		} else {
			where += fmt.Sprintf(" AND (i.invoice_number ILIKE $%d OR i.payer_email ILIKE $%d OR u.email ILIKE $%d)", argCount, argCount, argCount)
		}
		args = append(args, "%"+search+"%")
	}

	var total int
	if err := as.postgres.QueryRow(ctx, countQuery+where, args...).Scan(&total); err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusInternalServerError, Message: "failed to count invoices"}
	}

	argCount++
	query := baseQuery + where + fmt.Sprintf(" ORDER BY i.created_at DESC LIMIT $%d", argCount)
	args = append(args, limit)
	argCount++
	query += fmt.Sprintf(" OFFSET $%d", argCount)
	args = append(args, offset)

	rows, err := as.postgres.Query(ctx, query, args...)
	if err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusInternalServerError, Message: "failed to list invoices"}
	}
	defer rows.Close()

	invoices := []AdminInvoiceRow{}
	for rows.Next() {
		var inv AdminInvoiceRow
		if err := rows.Scan(&inv.ID, &inv.InvoiceNumber, &inv.CreatorEmail, &inv.PayerEmail, &inv.Total, &inv.Currency, &inv.Status, &inv.CreatedAt); err != nil {
			continue
		}
		invoices = append(invoices, inv)
	}

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "successful",
		Data: map[string]any{
			"invoices": invoices,
			"meta": AdminPaginationMeta{
				Page:       page,
				Limit:      limit,
				Total:      total,
				TotalPages: int(math.Ceil(float64(total) / float64(limit))),
			},
		},
	}
}
