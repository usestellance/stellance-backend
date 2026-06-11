package activitylog

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	ActionLogin          = "login"
	ActionInvoiceCreated = "invoice.created"
	ActionInvoiceSent    = "invoice.sent"
	ActionInvoicePaid    = "invoice.paid"
	ActionAdminSuspend   = "admin.suspend"
	ActionAdminActivate  = "admin.activate"
	ActionAdminDeactivate = "admin.deactivate"
	ActionAdminDelete    = "admin.delete"
	ActionPasswordReset  = "admin.password_reset"

	EntityInvoice = "invoice"
	EntityUser    = "user"
)

func Log(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger, userID, action, entityType, entityID, ipAddress string) {
	go func() {
		_, err := pool.Exec(context.Background(),
			`INSERT INTO user_activity_logs (user_id, action, entity_type, entity_id, ip_address)
			 VALUES ($1, $2, NULLIF($3,''), NULLIF($4,'')::uuid, NULLIF($5,''))`,
			userID, action, entityType, entityID, ipAddress,
		)
		if err != nil && log != nil {
			log.Warn("failed to write activity log", "error", err, "action", action, "user_id", userID)
		}
	}()
}
