package recurring

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/The-True-Hooha/stellance-backend/internal/invoice"
	"github.com/The-True-Hooha/stellance-backend/pkg/config"
	jwt_ "github.com/The-True-Hooha/stellance-backend/pkg/jwt"
	"github.com/The-True-Hooha/stellance-backend/pkg/utils"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type RecurringService struct {
	log      *slog.Logger
	postgres *pgxpool.Pool
	redis    *redis.Client
	jwt      *jwt_.JwtTokenServiceConfig
}

func NewRecurringService() *RecurringService {
	return &RecurringService{
		log:      config.GetAppContainer().Log,
		postgres: config.GetAppContainer().Postgres,
		redis:    config.GetAppContainer().Redis,
		jwt:      jwt_.JwtTokenService(),
	}
}

func nextRunTime(interval RecurringInterval, from time.Time) time.Time {
	switch interval {
	case IntervalWeekly:
		return from.AddDate(0, 0, 7)
	case IntervalBiweekly:
		return from.AddDate(0, 0, 14)
	case IntervalMonthly:
		return from.AddDate(0, 1, 0)
	case IntervalQuarterly:
		return from.AddDate(0, 3, 0)
	case IntervalYearly:
		return from.AddDate(1, 0, 0)
	default:
		return from.AddDate(0, 1, 0)
	}
}

func calculateTotals(items []invoice.InvoiceItems, serviceFee float64) (float64, float64, float64) {
	var subtotal float64
	for _, item := range items {
		subtotal += item.Amount
	}
	fee := subtotal * serviceFee / 100
	return subtotal, fee, subtotal + fee
}

func (rs *RecurringService) Create(ctx context.Context, userID string, dto CreateRecurringDTO) *utils.ApiResponse {
	startDate, err := time.Parse("2006-01-02", dto.StartDate)
	if err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusBadRequest, Message: "invalid start_date format"}
	}

	subtotal, fee, total := calculateTotals(dto.InvoiceItems, dto.ServiceFee)

	itemsJSON, err := json.Marshal(dto.InvoiceItems)
	if err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusInternalServerError, Message: "failed to encode invoice items"}
	}

	var id string
	err = rs.postgres.QueryRow(ctx, `
		INSERT INTO recurring_invoices (user_id, title, payer_email, payer_name, country, currency, sub_total, service_fee, total, template_id, note, interval, next_run_at, invoice_items)
		VALUES ($1, $2, $3, $4, $5, 'usdc', $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id
	`, userID, dto.Title, dto.PayerEmail, dto.PayerName, dto.Country, subtotal, fee, total, dto.TemplateID, dto.Note, string(dto.Interval), startDate, itemsJSON).Scan(&id)
	if err != nil {
		rs.log.Error("failed to create recurring invoice", "error", err)
		return &utils.ApiResponse{StatusCode: http.StatusInternalServerError, Message: "failed to create recurring invoice"}
	}

	return &utils.ApiResponse{
		StatusCode: http.StatusCreated,
		Message:    "recurring invoice created",
		Data:       map[string]any{"id": id, "next_run_at": startDate},
	}
}

func (rs *RecurringService) List(ctx context.Context, userID string) *utils.ApiResponse {
	rows, err := rs.postgres.Query(ctx, `
		SELECT id, user_id, title, payer_email, payer_name, country, currency, sub_total, service_fee, total, template_id, note, interval, next_run_at, last_run_at, is_active, invoice_items, created_at, updated_at
		FROM recurring_invoices WHERE user_id = $1 ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusInternalServerError, Message: "failed to list recurring invoices"}
	}
	defer rows.Close()

	var list []RecurringInvoice
	for rows.Next() {
		r, err := scanRecurring(rows)
		if err != nil {
			rs.log.Error("failed to scan recurring invoice", "error", err)
			continue
		}
		list = append(list, *r)
	}
	return &utils.ApiResponse{StatusCode: http.StatusOK, Message: "successful", Data: list}
}

func (rs *RecurringService) Get(ctx context.Context, id, userID string) *utils.ApiResponse {
	row := rs.postgres.QueryRow(ctx, `
		SELECT id, user_id, title, payer_email, payer_name, country, currency, sub_total, service_fee, total, template_id, note, interval, next_run_at, last_run_at, is_active, invoice_items, created_at, updated_at
		FROM recurring_invoices WHERE id = $1 AND user_id = $2
	`, id, userID)
	r, err := scanRecurringRow(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return &utils.ApiResponse{StatusCode: http.StatusNotFound, Message: "not found"}
		}
		return &utils.ApiResponse{StatusCode: http.StatusInternalServerError, Message: "failed to fetch recurring invoice"}
	}
	return &utils.ApiResponse{StatusCode: http.StatusOK, Message: "successful", Data: r}
}

func (rs *RecurringService) Update(ctx context.Context, id, userID string, dto UpdateRecurringDTO) *utils.ApiResponse {
	existing := rs.Get(ctx, id, userID)
	if existing.StatusCode != http.StatusOK {
		return existing
	}
	r := existing.Data.(*RecurringInvoice)

	isActive := r.IsActive
	if dto.IsActive != nil {
		isActive = *dto.IsActive
	}
	interval := r.Interval
	if dto.Interval != "" {
		interval = dto.Interval
	}
	nextRun := r.NextRunAt
	if dto.StartDate != "" {
		t, err := time.Parse("2006-01-02", dto.StartDate)
		if err != nil {
			return &utils.ApiResponse{StatusCode: http.StatusBadRequest, Message: "invalid start_date"}
		}
		nextRun = t
	}

	_, err := rs.postgres.Exec(ctx, `
		UPDATE recurring_invoices SET is_active = $1, interval = $2, next_run_at = $3, updated_at = NOW()
		WHERE id = $4 AND user_id = $5
	`, isActive, string(interval), nextRun, id, userID)
	if err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusInternalServerError, Message: "failed to update recurring invoice"}
	}
	return &utils.ApiResponse{StatusCode: http.StatusOK, Message: "recurring invoice updated"}
}

func (rs *RecurringService) Delete(ctx context.Context, id, userID string) *utils.ApiResponse {
	result, err := rs.postgres.Exec(ctx, `DELETE FROM recurring_invoices WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusInternalServerError, Message: "failed to delete recurring invoice"}
	}
	if result.RowsAffected() == 0 {
		return &utils.ApiResponse{StatusCode: http.StatusNotFound, Message: "not found"}
	}
	return &utils.ApiResponse{StatusCode: http.StatusOK, Message: "deleted"}
}

func (rs *RecurringService) GenerateDue(ctx context.Context) error {
	rows, err := rs.postgres.Query(ctx, `
		SELECT id, user_id, title, payer_email, payer_name, country, currency, sub_total, service_fee, total, template_id, note, interval, invoice_items
		FROM recurring_invoices
		WHERE is_active = TRUE AND next_run_at <= NOW()
	`)
	if err != nil {
		return fmt.Errorf("failed to query due recurring invoices: %w", err)
	}
	defer rows.Close()

	type dueRow struct {
		id, userID, title, payerEmail, payerName, country, currency, templateID, note string
		subTotal, serviceFee, total                                                    float64
		interval                                                                       RecurringInterval
		itemsJSON                                                                      []byte
	}

	var due []dueRow
	for rows.Next() {
		var d dueRow
		if err := rows.Scan(&d.id, &d.userID, &d.title, &d.payerEmail, &d.payerName, &d.country,
			&d.currency, &d.subTotal, &d.serviceFee, &d.total, &d.templateID, &d.note, &d.interval, &d.itemsJSON); err != nil {
			rs.log.Error("failed to scan due recurring invoice", "error", err)
			continue
		}
		due = append(due, d)
	}

	invoiceSvc := invoice.NewInvoiceService()
	for _, d := range due {
		var items []invoice.InvoiceItems
		if err := json.Unmarshal(d.itemsJSON, &items); err != nil {
			rs.log.Error("failed to unmarshal items for recurring invoice", "id", d.id, "error", err)
			continue
		}

		dueDate := nextRunTime(d.interval, time.Now()).Format("2006-01-02")
		dto := invoice.CreateInvoiceDTO{
			Title:         d.title,
			RecipientName: d.payerName,
			Email:         d.payerEmail,
			Country:       d.country,
			InvoiceItems:  items,
			ServiceFee:    d.serviceFee,
			DueDate:       dueDate,
			TemplateID:    invoice.TemplateIDType(d.templateID),
			Note:          d.note,
		}

		resp := invoiceSvc.GenerateNewInvoice(ctx, dto, d.userID, nil)
		if resp.StatusCode != http.StatusCreated {
			rs.log.Error("failed to generate invoice from recurring", "recurring_id", d.id, "error", resp.Message)
			continue
		}

		next := nextRunTime(d.interval, time.Now())
		rs.postgres.Exec(ctx, `
			UPDATE recurring_invoices SET last_run_at = NOW(), next_run_at = $1 WHERE id = $2
		`, next, d.id)
		rs.log.Info("generated invoice from recurring schedule", "recurring_id", d.id, "next_run", next)
	}
	return nil
}

func scanRecurring(rows pgx.Rows) (*RecurringInvoice, error) {
	var r RecurringInvoice
	var itemsJSON []byte
	var note, payerName, country, templateID *string
	if err := rows.Scan(&r.ID, &r.UserID, &r.Title, &r.PayerEmail, &payerName, &country,
		&r.Currency, &r.SubTotal, &r.ServiceFee, &r.Total, &templateID, &note,
		&r.Interval, &r.NextRunAt, &r.LastRunAt, &r.IsActive, &itemsJSON, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, err
	}
	if note != nil {
		r.Note = *note
	}
	if payerName != nil {
		r.PayerName = *payerName
	}
	if country != nil {
		r.Country = *country
	}
	if templateID != nil {
		r.TemplateID = *templateID
	}
	json.Unmarshal(itemsJSON, &r.InvoiceItems)
	return &r, nil
}

func scanRecurringRow(row pgx.Row) (*RecurringInvoice, error) {
	var r RecurringInvoice
	var itemsJSON []byte
	var note, payerName, country, templateID *string
	if err := row.Scan(&r.ID, &r.UserID, &r.Title, &r.PayerEmail, &payerName, &country,
		&r.Currency, &r.SubTotal, &r.ServiceFee, &r.Total, &templateID, &note,
		&r.Interval, &r.NextRunAt, &r.LastRunAt, &r.IsActive, &itemsJSON, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, err
	}
	if note != nil {
		r.Note = *note
	}
	if payerName != nil {
		r.PayerName = *payerName
	}
	if country != nil {
		r.Country = *country
	}
	if templateID != nil {
		r.TemplateID = *templateID
	}
	json.Unmarshal(itemsJSON, &r.InvoiceItems)
	return &r, nil
}
