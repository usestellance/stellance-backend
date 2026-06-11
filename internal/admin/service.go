package admin

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"math"
	"math/big"
	"net/http"
	"net/url"
	"time"

	"github.com/The-True-Hooha/stellance-backend/internal/activitylog"
	"github.com/The-True-Hooha/stellance-backend/mail"
	"github.com/The-True-Hooha/stellance-backend/pkg/config"
	jwt_ "github.com/The-True-Hooha/stellance-backend/pkg/jwt"
	"github.com/The-True-Hooha/stellance-backend/pkg/utils"
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
	ID            string     `json:"id"`
	Email         string     `json:"email"`
	FirstName     string     `json:"first_name"`
	LastName      string     `json:"last_name"`
	Role          string     `json:"role"`
	EmailVerified bool       `json:"email_verified"`
	IsActive      bool       `json:"is_active"`
	CreatedAt     time.Time  `json:"created_at"`
}

type AdminInvoiceRow struct {
	ID            string    `json:"id"`
	InvoiceNumber string    `json:"invoice_number"`
	CreatorEmail  string    `json:"creator_email"`
	PayerEmail    string    `json:"payer_email"`
	Total         float64   `json:"total"`
	Currency      string    `json:"currency"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
}

type AdminStats struct {
	TotalUsers    int     `json:"total_users"`
	ActiveUsers   int     `json:"active_users"`
	TotalInvoices int     `json:"total_invoices"`
	PaidInvoices  int     `json:"paid_invoices"`
	OverdueCount  int     `json:"overdue_count"`
	TotalRevenue  float64 `json:"total_revenue"`
}

type AdminPaginationMeta struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
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

	var users []AdminUserRow
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
		`SELECT t.id, COALESCE(i.invoice_number,''), u.email, t.amount, t.transaction_type::text, t.status::text, t.created_at
		 FROM transactions t
		 JOIN users u ON u.id = t.user_id
		 LEFT JOIN invoice i ON i.id = t.invoice_id
		 WHERE t.user_id = $1 ORDER BY t.created_at DESC LIMIT 10`, userID,
	)
	var recentTx []AdminTransactionRow
	if txRows != nil {
		defer txRows.Close()
		for txRows.Next() {
			var tx AdminTransactionRow
			if err := txRows.Scan(&tx.ID, &tx.InvoiceNumber, &tx.UserEmail, &tx.Amount, &tx.Type, &tx.Status, &tx.CreatedAt); err == nil {
				recentTx = append(recentTx, tx)
			}
		}
	}

	logRows, _ := as.postgres.Query(ctx,
		`SELECT id, user_id, action, COALESCE(entity_type,''), COALESCE(entity_id::text,''), COALESCE(ip_address,''), created_at
		 FROM user_activity_logs WHERE user_id = $1 ORDER BY created_at DESC LIMIT 20`, userID,
	)
	var recentActivity []ActivityLog
	if logRows != nil {
		defer logRows.Close()
		for logRows.Next() {
			var l ActivityLog
			if err := logRows.Scan(&l.ID, &l.UserID, &l.Action, &l.EntityType, &l.EntityID, &l.IPAddress, &l.CreatedAt); err == nil {
				recentActivity = append(recentActivity, l)
			}
		}
	}

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "successful",
		Data: map[string]any{
			"user":            u,
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
	ID            string    `json:"id"`
	InvoiceNumber string    `json:"invoice_number"`
	UserEmail     string    `json:"user_email"`
	Amount        float64   `json:"amount"`
	Type          string    `json:"type"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
}

func (as *AdminService) ListTransactions(ctx context.Context, page, limit int, search string) *utils.ApiResponse {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	base := `SELECT t.id, COALESCE(i.invoice_number,''), u.email, t.amount, t.transaction_type::text, t.status::text, t.created_at
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

	var txs []AdminTransactionRow
	for rows.Next() {
		var tx AdminTransactionRow
		if err := rows.Scan(&tx.ID, &tx.InvoiceNumber, &tx.UserEmail, &tx.Amount, &tx.Type, &tx.Status, &tx.CreatedAt); err != nil {
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
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	Action     string    `json:"action"`
	EntityType string    `json:"entity_type,omitempty"`
	EntityID   string    `json:"entity_id,omitempty"`
	Metadata   any       `json:"metadata,omitempty"`
	IPAddress  string    `json:"ip_address,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
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

	var invoices []AdminInvoiceRow
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
		`SELECT t.id, COALESCE(i.invoice_number,''), u.email, t.amount, t.transaction_type::text, t.status::text, t.created_at
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

	var txs []AdminTransactionRow
	for rows.Next() {
		var tx AdminTransactionRow
		if err := rows.Scan(&tx.ID, &tx.InvoiceNumber, &tx.UserEmail, &tx.Amount, &tx.Type, &tx.Status, &tx.CreatedAt); err != nil {
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

func (as *AdminService) SuspendUser(ctx context.Context, userID, adminID, ip string) *utils.ApiResponse {
	var current bool
	if err := as.postgres.QueryRow(ctx, `SELECT is_active FROM users WHERE id = $1`, userID).Scan(&current); err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusNotFound, Message: "user not found"}
	}
	newState := !current
	as.postgres.Exec(ctx, `UPDATE users SET is_active = $1, updated_at = NOW() WHERE id = $2`, newState, userID)
	action := activitylog.ActionAdminSuspend
	msg := "user suspended"
	if newState {
		action = activitylog.ActionAdminActivate
		msg = "user unsuspended"
	}
	as.log_(ctx, adminID, action, activitylog.EntityUser, userID, ip)
	return &utils.ApiResponse{StatusCode: http.StatusOK, Message: msg, Data: map[string]any{"is_active": newState}}
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

	var logs []ActivityLog
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

	var invoices []AdminInvoiceRow
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
