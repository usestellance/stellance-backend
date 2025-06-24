package invoice

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/The-True-Hooha/stellance-backend.git/internal/user"
	"github.com/The-True-Hooha/stellance-backend.git/pkg/config"
	jwt_ "github.com/The-True-Hooha/stellance-backend.git/pkg/jwt"
	"github.com/The-True-Hooha/stellance-backend.git/pkg/utils"
	"github.com/jackc/pgx"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const (
	SERVICE_FEE_PERCENTAGE = 2.5
)

type InvoiceService struct {
	log      *slog.Logger
	postgres *pgxpool.Pool
	redis    *redis.Client
	jwt      *jwt_.JwtTokenServiceConfig
}

func NewInvoiceService() *InvoiceService {
	return &InvoiceService{
		log:      config.GetAppContainer().Log,
		postgres: config.GetAppContainer().Postgres,
		redis:    config.GetAppContainer().Redis,
		jwt:      jwt_.JwtTokenService(),
	}
}

func (is *InvoiceService) GenerateNewInvoice(ctx context.Context, dto CreateInvoiceDTO, userId string) *utils.ApiResponse {
	subtotal, serviceFee, total, err := is.validateAndCalculateInvoice(dto)
	if err != nil {
		is.log.Error("error trying to calculate invoice numbers", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		}
	}

	tx, err := is.postgres.Begin(ctx)
	if err != nil {
		is.log.Error("failed to begin transaction", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to process request. Please try again.",
		}
	}
	defer tx.Rollback(ctx)

	var businessName sql.NullString

	const businessNameQ string = `SELECT business_name FROM users WHERE id = $1
		AND is_active = true
		AND email_verified = true
		AND first_name IS NOT NULL 
		AND first_name <> '' 
		AND last_name IS NOT NULL 
		AND last_name <> ''`
	err = tx.QueryRow(ctx, businessNameQ, userId).Scan(&businessName)
	if err != nil {
		if err == pgx.ErrNoRows {
			return &utils.ApiResponse{
				StatusCode: http.StatusForbidden,
				Message:    "Please contact support, your profile is not yet complete",
			}
		}
		is.log.Error("failed to fetch user", "error", err, "user_id", userId)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to process request. Please try again.",
		}
	}

	businessNameStr := ""
	if businessName.Valid {
		businessNameStr = businessName.String
	}

	invoiceNumber, err := is.GenerateAndFormatInvoiceNumber(ctx, userId, businessNameStr)
	if err != nil {
		is.log.Error("failed to generate invoice number", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to process request. Please try again.",
		}
	}

	invoice_url, err := utils.GenerateShortURL(fmt.Sprintf("%s%s", invoiceNumber, userId), is.log)
	if err != nil {
		is.log.Error("failed to generate invoice url", "error", err)
		return &utils.ApiResponse{
			Message:    "failed to generate invoice number",
			StatusCode: http.StatusInternalServerError,
		}
	}

	dueDate, err := time.Parse("2006-01-02", dto.DueDate)
	if err != nil {
		return &utils.ApiResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "Invalid due date format. Use YYYY-MM-DD",
		}
	}
	const invoiceQ = `
	INSERT INTO invoice(invoice_number, invoice_url, created_by_id, payer_email, 
	sub_total, service_fee, total, currency, title, status,due_date, address_country)
	VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)RETURNING id, created_at`

	var invoiceId string
	var createdAt time.Time

	err = tx.QueryRow(ctx, invoiceQ, invoiceNumber, invoice_url, userId, dto.Email, subtotal, serviceFee, total, utils.USDC, dto.Title, InvoiceStatusDraft, dueDate, dto.Country).Scan(&invoiceId, &createdAt)
	if err != nil {
		is.log.Error("failed to create invoice", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to create invoice. Please try again.",
		}
	}

	const itemQ = `
		INSERT INTO invoice_items (
		invoice_id, item_type, description,quantity, unit_price, discount, amount
	) VALUES($1, $2, $3, $4, $5, $6, $7)`
	for _, item := range dto.InvoiceItems {
		_, err = tx.Exec(ctx, itemQ,
			invoiceId,
			item.InvoiceType,
			item.Description,
			item.Quantity,
			item.UnitPrice,
			item.Discount,
			item.Amount,
		)

		if err != nil {
			is.log.Error("failed to create invoice item", "error", err, "invoice_id", invoiceId)
			return &utils.ApiResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    "Failed to create invoice items. Please try again.",
			}

		}
	}

	if err = tx.Commit(ctx); err != nil {
		is.log.Error("failed to commit transaction", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to complete invoice creation. Please try again.",
		}
	}

	invoiceResponse := InvoiceResponse{
		ID:            invoiceId,
		InvoiceNumber: invoiceNumber,
		InvoiceURL:    invoice_url,
		Title:         dto.Title,
		PayerEmail:    dto.Email,
		PayerName:     dto.RecipientName,
		Country:       dto.Country,
		SubTotal:      subtotal,
		ServiceFee:    dto.ServiceFee,
		Total:         total,
		Currency:      string(utils.USDC),
		Status:        string(InvoiceStatusDraft),
		DueDate:       dueDate.Format("2006-01-02"),
		CreatedAt:     createdAt,
		Items:         dto.InvoiceItems,
	}

	is.log.Info("invoice created successfully",
		"invoice_id", invoiceId,
		"invoice_number", invoiceNumber,
		"user_id", userId,
	)

	return &utils.ApiResponse{
		StatusCode: http.StatusCreated,
		Message:    "Invoice created successfully",
		Data:       invoiceResponse,
	}

}

func (s *InvoiceService) GenerateInvoiceNumber(ctx context.Context, userID string) (string, error) {
	year := time.Now().Year()
	var invoiceNumber int

	const query = `
		INSERT INTO invoice_counters (user_id, year, last_number)
		VALUES ($1, $2, 1)
		ON CONFLICT (user_id, year)
		DO UPDATE SET 
			last_number = invoice_counters.last_number + 1,
			updated_at = NOW()
		RETURNING last_number
	`

	err := s.postgres.QueryRow(ctx, query, userID, year).Scan(&invoiceNumber)
	if err != nil {
		return "", fmt.Errorf("failed to generate invoice number: %w", err)
	}
	return fmt.Sprintf("INV-%d-%04d", year, invoiceNumber), nil
}

func (is *InvoiceService) GenerateAndFormatInvoiceNumber(ctx context.Context, userId, businessName string) (string, error) {
	num, err := is.GenerateInvoiceNumber(ctx, userId)
	if err != nil {
		return "", err
	}

	prefix := "INV"
	if businessName != "" && len(businessName) >= 3 {
		prefix = strings.ToUpper(businessName[:3])
	}
	year := time.Now().Year()
	userSuffix := userId[:4]
	is.log.Debug("new invoice number generated for ")
	invoiceNumber := fmt.Sprintf("%s-%d-%s-%s", prefix, year, num, userSuffix)
	is.log.Debug("new invoice number generated", "invoice_number", invoiceNumber, "userId", userId)
	return invoiceNumber, nil
}

func (is *InvoiceService) validateAndCalculateInvoice(dto CreateInvoiceDTO) (subtotal, serviceFee, total float64, err error) {
	for _, item := range dto.InvoiceItems {
		itemAmount := float64(item.Quantity) * item.UnitPrice
		if item.Discount > 0 {
			itemAmount = itemAmount - (itemAmount * float64(item.Discount) / 100)
		}
		if math.Abs(itemAmount-item.Amount) > 0.01 {
			return 0, 0, 0, fmt.Errorf("invalid amount for item '%s': expected %.2f, got %.2f",
				item.Description, itemAmount, item.Amount)
		}
		subtotal += item.Amount
	}
	serviceFee = subtotal * (SERVICE_FEE_PERCENTAGE / 100)
	serviceFee = math.Round(serviceFee*100) / 100
	total = subtotal + serviceFee
	return subtotal, serviceFee, total, nil
}

func (is *InvoiceService) GetManyInvoice(ctx context.Context, dto InvoiceFiltersDto, user_id string) *utils.ApiResponse {
	if dto.Page < 1 {
		dto.Page = 1
	}
	if dto.Count < 1 || dto.Count > 10 {
		dto.Count = 10
	}

	if dto.OrderBy == "" {
		dto.OrderBy = utils.OrderByDESC
	}

	if dto.Status != "" {
		validStatuses := map[InvoiceStatus]bool{
			InvoiceStatusDraft:     true,
			InvoiceStatusSent:      true,
			InvoiceStatusPaid:      true,
			InvoiceStatusOverdue:   true,
			InvoiceStatusCancelled: true,
			InvoiceStatusRefunded:  true,
			InvoiceStatusViewed:    true,
		}
		if !validStatuses[dto.Status] {
			return &utils.ApiResponse{
				StatusCode: http.StatusBadRequest,
				Message:    fmt.Sprintf("Invalid status: %s", dto.Status),
			}
		}
	}

	query, args := is.buildInvoiceQuery(dto, user_id)

	countQuery := `
		SELECT COUNT(*)
		FROM invoice i
		WHERE i.created_by_id = $1
		%s
	`

	var whereClause string
	countArgs := []interface{}{user_id}
	argCount := 1
	if dto.Status != "" {
		argCount++
		whereClause += fmt.Sprintf(" AND i.status = $%d", argCount)
		countArgs = append(countArgs, dto.Status)
	}

	var totalItems int
	err := is.postgres.QueryRow(ctx, fmt.Sprintf(countQuery, whereClause), countArgs...).Scan(&totalItems)
	if err != nil {
		is.log.Error("failed to get invoice count", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to fetch invoices",
		}
	}

	rows, err := is.postgres.Query(ctx, query, args...)
	if err != nil {
		is.log.Error("failed to fetch invoices", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to fetch invoices",
		}
	}
	defer rows.Close()

	invoices := []InvoiceResponse{}
	for rows.Next() {
		var invoice InvoiceResponse
		var payerName sql.NullString
		var title sql.NullString
		var paidAt sql.NullTime
		var Country sql.NullString

		err := rows.Scan(
			&invoice.ID,
			&invoice.InvoiceNumber,
			&invoice.InvoiceURL,
			&title,
			&invoice.PayerEmail,
			&payerName,
			&invoice.SubTotal,
			&invoice.ServiceFee,
			&invoice.Total,
			&invoice.Currency,
			&invoice.Status,
			&invoice.DueDate,
			&paidAt,
			&invoice.CreatedAt,
			&invoice.UpdatedAt,
			&Country,
		)
		if err != nil {
			is.log.Error("failed to retrieve invoice", "error", err)
			continue
		}

		if title.Valid {
			invoice.Title = title.String
		}
		if payerName.Valid {
			invoice.PayerName = payerName.String
		}
		if paidAt.Valid {
			invoice.PaidAt = &paidAt.Time
		}

		invoices = append(invoices, invoice)
	}
	totalPages := int(math.Ceil(float64(totalItems) / float64(dto.Count)))
	response := InvoiceListResponseDto{
		Invoice: invoices,
		Meta: PaginationMeta{
			Page:       dto.Page,
			Count:      dto.Count,
			TotalItems: totalItems,
			TotalPages: totalPages,
		},
	}
	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "successful",
		Data:       response,
	}
}

func (is *InvoiceService) buildInvoiceQuery(filters InvoiceFiltersDto, userId string) (string, []interface{}) {
	offset := (filters.Page - 1) * filters.Count

	query := `
		SELECT 
			i.id,
			i.invoice_number,
			i.invoice_url,
			i.title,
			i.payer_email,
			i.payer_name,
			i.sub_total,
			i.service_fee,
			i.total,
			i.currency,
			i.status,
			i.due_date,
			i.paid_at,
			i.created_at,
			i.updated_at,
			i.address_country
		FROM invoice i
		WHERE i.created_by_id = $1
	`
	args := []interface{}{userId}
	argCount := 1

	if filters.Status != "" {
		argCount++
		query += fmt.Sprintf(" AND i.status = $%d", argCount)
		args = append(args, filters.Status)
		if filters.Status == "overdue" {
			query += " AND i.due_date < CURRENT_DATE AND i.status != 'paid'"
		}
	}

	query += fmt.Sprintf("ORDER BY i.created_at %s", filters.OrderBy)

	argCount++
	query += fmt.Sprintf(" LIMIT $%d", argCount)
	args = append(args, filters.Count)

	argCount++
	query += fmt.Sprintf(" OFFSET $%d", argCount)
	args = append(args, offset)

	return query, args
}

func (is *InvoiceService) GetInvoiceById(ctx context.Context, invoiceId, userId, role string) *utils.ApiResponse {
	log := is.log

	const checkQ = `
		SELECT created_by_id
		FROM invoice
		WHERE id = $1
	`
	var invoice_owner string
	err := is.postgres.QueryRow(ctx, checkQ, invoiceId).Scan(&invoice_owner)
	if err != nil {
		if err == pgx.ErrNoRows {
			return &utils.ApiResponse{
				StatusCode: http.StatusNotFound,
				Message:    "Invoice data not found",
			}
		}
		log.Error("failed to to get user invoice", "error", err, "user_id", userId, "invoice_id", invoiceId)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to get invoice",
		}
	}
	if userId != invoice_owner && role != string(user.RoleAdmin) {
		return &utils.ApiResponse{
			StatusCode: http.StatusForbidden,
			Message:    "Invoice data not found",
		}
	}

	const query = `
		SELECT
			id,
			invoice_number,
			invoice_url,
			title,
			payer_email,
			payer_name,
			sub_total,
			service_fee,
			total,
			currency,
			status,
			due_date,
			paid_at,
			created_at,
			updated_at,
			address_country
		FROM invoice WHERE id = $1 AND created_by_id = $2
	`
	var invoice InvoiceResponse
	var payerName sql.NullString
	var title sql.NullString
	var paidAt sql.NullTime
	var Country sql.NullString

	err = is.postgres.QueryRow(ctx, query, invoiceId, userId).Scan(
		&invoice.ID,
		&invoice.InvoiceNumber,
		&invoice.InvoiceURL,
		&title,
		&invoice.PayerEmail,
		&payerName,
		&invoice.SubTotal,
		&invoice.ServiceFee,
		&invoice.Total,
		&invoice.Currency,
		&invoice.Status,
		&invoice.DueDate,
		&paidAt,
		&invoice.CreatedAt,
		&invoice.UpdatedAt,
		&Country,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return &utils.ApiResponse{
				StatusCode: http.StatusNotFound,
				Message:    "Invoice data not found",
			}
		}
		log.Error("failed to to get user invoice", "error", err, "user_id", userId, "invoice_id", invoiceId)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to get invoice",
		}
	}

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "successful",
		Data:       invoice,
	}
}

func (is *InvoiceService) GetInvoiceByUrl(ctx context.Context, invoiceUrl, userId, role string) *utils.ApiResponse {
	log := is.log

	const checkQ = `
		SELECT created_by_id
		FROM invoice
		WHERE invoice_url = $1
	`
	var invoice_owner string
	err := is.postgres.QueryRow(ctx, checkQ, invoiceUrl).Scan(&invoice_owner)
	if err != nil {
		if err == pgx.ErrNoRows {
			return &utils.ApiResponse{
				StatusCode: http.StatusNotFound,
				Message:    "Invoice data not found",
			}
		}
		log.Error("failed to to get user invoice", "error", err, "user_id", userId, "invoice_url", invoiceUrl)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to get invoice",
		}
	}
	if userId != invoice_owner && role != string(user.RoleAdmin) {
		return &utils.ApiResponse{
			StatusCode: http.StatusForbidden,
			Message:    "Invoice data not found",
		}
	}

	const query = `
		SELECT
			id,
			invoice_number,
			invoice_url,
			title,
			payer_email,
			payer_name,
			sub_total,
			service_fee,
			total,
			currency,
			status,
			due_date,
			paid_at,
			created_at,
			updated_at,
			address_country
		FROM invoice WHERE invoice_url = $1 AND created_by_id = $2
	`
	var invoice InvoiceResponse
	var payerName sql.NullString
	var title sql.NullString
	var paidAt sql.NullTime
	var Country sql.NullString

	err = is.postgres.QueryRow(ctx, query, invoiceUrl, userId).Scan(
		&invoice.ID,
		&invoice.InvoiceNumber,
		&invoice.InvoiceURL,
		&title,
		&invoice.PayerEmail,
		&payerName,
		&invoice.SubTotal,
		&invoice.ServiceFee,
		&invoice.Total,
		&invoice.Currency,
		&invoice.Status,
		&invoice.DueDate,
		&paidAt,
		&invoice.CreatedAt,
		&invoice.UpdatedAt,
		&Country,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return &utils.ApiResponse{
				StatusCode: http.StatusNotFound,
				Message:    "Invoice data not found",
			}
		}
		log.Error("failed to to get user invoice", "error", err, "user_id", userId, "invoice_url", invoiceUrl)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to get invoice",
		}
	}

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "successful",
		Data:       invoice,
	}
}
