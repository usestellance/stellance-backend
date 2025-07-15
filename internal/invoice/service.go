package invoice

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/The-True-Hooha/stellance-backend.git/internal/user"
	"github.com/The-True-Hooha/stellance-backend.git/mail"
	"github.com/The-True-Hooha/stellance-backend.git/pkg/config"
	jwt_ "github.com/The-True-Hooha/stellance-backend.git/pkg/jwt"
	"github.com/The-True-Hooha/stellance-backend.git/pkg/utils"
	"github.com/google/uuid"
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
	mail     *mail.Mailer
}

func NewInvoiceService() *InvoiceService {
	return &InvoiceService{
		log:      config.GetAppContainer().Log,
		postgres: config.GetAppContainer().Postgres,
		redis:    config.GetAppContainer().Redis,
		jwt:      jwt_.JwtTokenService(),
		mail:     mail.NewMailer(),
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
	var country string
	var first_name string
	var last_name string
	var email string

	const businessNameQ string = `SELECT business_name, country, email, first_name, last_name FROM users WHERE id = $1
		AND is_active = true
		AND email_verified = true
		AND first_name IS NOT NULL 
		AND first_name <> '' 
		AND last_name IS NOT NULL 
		AND last_name <> ''`
	err = tx.QueryRow(ctx, businessNameQ, userId).Scan(&businessName, &country, &email, &first_name, &last_name)
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

	invoiceNumber, err := is.GenerateInvoiceNumber(ctx, userId)
	fmt.Println("the invoice number key", invoiceNumber)
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
	sub_total, service_fee, total, currency, title, status,due_date, address_country, payer_name)
	VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)RETURNING id, created_at`

	var invoiceId string
	var createdAt time.Time

	err = tx.QueryRow(ctx, invoiceQ, invoiceNumber, invoice_url, userId, dto.Email, subtotal, serviceFee, total, utils.USDC, dto.Title, InvoiceStatusDraft, dueDate, dto.Country, dto.RecipientName).Scan(&invoiceId, &createdAt)
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

	sender := InvoiceSenderDetails{
		UserId:       userId,
		BusinessName: &businessNameStr,
		Name:         first_name + " " + last_name,
		Email:        email,
		Location:     country,
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
		DueDate:       dueDate,
		CreatedAt:     createdAt,
		Items:         dto.InvoiceItems,
		CreatedBy:     sender,
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
	shortUserID := userID[:8]
	
	_, err := s.postgres.Exec(ctx, `
		INSERT INTO invoice_counters (user_id, year, last_number)
		VALUES ($1, $2, 0)
		ON CONFLICT (user_id, year) DO NOTHING
	`, userID, year)
	if err != nil {
		return "", fmt.Errorf("failed to ensure counter exists: %w", err)
	}
	
	var invoiceNumber int
	err = s.postgres.QueryRow(ctx, `
		UPDATE invoice_counters 
		SET last_number = last_number + 1,
		    updated_at = NOW()
		WHERE user_id = $1 AND year = $2
		RETURNING last_number
	`, userID, year).Scan(&invoiceNumber)
	
	if err != nil {
		return "", fmt.Errorf("failed to generate invoice number: %w", err)
	}
	return fmt.Sprintf("INV-%s-%04d", shortUserID, invoiceNumber), nil
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
	if dto.Status != "" {
		validStatuses := map[InvoiceStatus]bool{
			InvoiceStatusDraft:     true,
			InvoiceStatusSent:      true,
			InvoiceStatusPaid:      true,
			InvoiceStatusOverdue:   true,
			InvoiceStatusCancelled: true,
			InvoiceStatusRefunded:  true,
			InvoiceStatusViewed:    true,
			InvoiceStatusPending:   true,
		}
		if !validStatuses[dto.Status] {
			return &utils.ApiResponse{
				StatusCode: http.StatusBadRequest,
				Message:    fmt.Sprintf("Invalid status: %s", dto.Status),
			}
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

	var bName sql.NullString
	var sender_country string
	var first_name string
	var last_name string
	var email string

	const qq string = `SELECT business_name, country, email,
		first_name, last_name FROM users WHERE id = $1
		AND is_active = true
		AND email_verified = true
		AND first_name IS NOT NULL 
		AND first_name <> '' 
		AND last_name IS NOT NULL 
		AND last_name <> ''
	`

	query, args := is.buildInvoiceQuery(dto, user_id)
	countQuery := `
		SELECT COUNT(*)
		FROM invoice i
		WHERE i.created_by_id = $1
		%s
	`
	err = tx.QueryRow(ctx, qq, user_id).Scan(&bName, &sender_country, &email, &first_name, &last_name)
	if err != nil {
		if err == pgx.ErrNoRows {
			return &utils.ApiResponse{
				StatusCode: http.StatusForbidden,
				Message:    "Please contact support, your profile is not yet complete",
			}
		}
		is.log.Error("failed to fetch user", "error", err, "user_id", user_id)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to process request. Please try again.",
		}
	}

	var whereClause string
	countArgs := []interface{}{user_id}
	argCount := 1
	if dto.Status != "" {
		argCount++
		whereClause += fmt.Sprintf(" AND i.status = $%d", argCount)
		countArgs = append(countArgs, dto.Status)
	}

	var totalItems int
	err = tx.QueryRow(ctx, fmt.Sprintf(countQuery, whereClause), countArgs...).Scan(&totalItems)
	if err != nil {
		is.log.Error("failed to get invoice count", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to fetch invoices",
		}
	}
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		is.log.Error("failed to fetch invoices", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to fetch invoices",
		}
	}
	defer rows.Close()

	invoices := []InvoiceResponse{}
	invoiceIDs := []string{}

	for rows.Next() {
		var invoice InvoiceResponse
		var payerWalletAddress sql.NullString
		var title sql.NullString
		var paidAt sql.NullTime
		var country sql.NullString

		err := rows.Scan(
			&invoice.ID,
			&invoice.InvoiceNumber,
			&invoice.InvoiceURL,
			&title,
			&invoice.PayerEmail,
			&invoice.PayerName,
			&payerWalletAddress,
			&invoice.SubTotal,
			&invoice.ServiceFee,
			&invoice.Total,
			&invoice.Currency,
			&invoice.Status,
			&invoice.DueDate,
			&paidAt,
			&invoice.CreatedAt,
			&invoice.UpdatedAt,
			&country,
		)
		if err != nil {
			is.log.Error("failed to scan invoice", "error", err)
			continue
		}

		if title.Valid {
			invoice.Title = title.String
		}
		if payerWalletAddress.Valid {
			invoice.PayerWalletAddress = payerWalletAddress.String
		}
		if paidAt.Valid {
			invoice.PaidAt = &paidAt.Time
		}
		if country.Valid {
			invoice.Country = country.String
		}

		sender := InvoiceSenderDetails{
			UserId:       user_id,
			Name:         first_name + " " + last_name,
			Email:        email,
			Location:     sender_country,
			BusinessName: &bName.String,
		}
		invoice.CreatedBy = sender
		invoices = append(invoices, invoice)
		invoiceIDs = append(invoiceIDs, invoice.ID)
	}

	itemMap, err := is.fetchAllInvoiceItems(ctx, invoiceIDs)
	if err != nil {
		is.log.Error("failed to fetch invoice items", "error", err)

	}

	for i := range invoices {
		if items, ok := itemMap[invoices[i].ID]; ok {
			invoices[i].Items = items
		} else {
			invoices[i].Items = []InvoiceItems{}
		}
	}

	totalPages := int(math.Ceil(float64(totalItems) / float64(dto.Count)))
	response := InvoiceListResponseDto{
		Invoice: invoices,
		Meta: PaginationMeta{
			Page:              dto.Page,
			PageCount:         dto.Count,
			TotalInvoiceCount: totalItems,
			TotalPages:        totalPages,
		},
	}

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "Invoices fetched successfully",
		Data:       response,
	}
}

func (is *InvoiceService) fetchAllInvoiceItems(ctx context.Context, invoiceIDs []string) (map[string][]InvoiceItems, error) {
	if len(invoiceIDs) == 0 {
		return map[string][]InvoiceItems{}, nil
	}

	const query string = `
		SELECT 
			invoice_id,
			id, 
			item_type, 
			description, 
			quantity, 
			unit_price, 
			discount, 
			amount, 
			created_at
		FROM invoice_items
		WHERE invoice_id = ANY($1)
		ORDER BY invoice_id, created_at
	`

	rows, err := is.postgres.Query(ctx, query, invoiceIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to query invoice items: %w", err)
	}
	defer rows.Close()

	itemMap := make(map[string][]InvoiceItems)

	for rows.Next() {
		var item InvoiceItems
		var invoiceID string

		err := rows.Scan(
			&invoiceID,
			&item.ItemId,
			&item.InvoiceType,
			&item.Description,
			&item.Quantity,
			&item.UnitPrice,
			&item.Discount,
			&item.Amount,
			&item.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan invoice item: %w", err)
		}

		itemMap[invoiceID] = append(itemMap[invoiceID], item)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating invoice items: %w", err)
	}

	return itemMap, nil
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
			i.payer_wallet_address,
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

	query += fmt.Sprintf(" ORDER BY i.created_at %s", filters.OrderBy)

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

	if _, err := uuid.Parse(invoiceId); err != nil {
		return &utils.ApiResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "Invalid invoice ID format",
		}
	}

	cacheKey := fmt.Sprintf("invoice:%s", invoiceId)
	if cached, err := is.getFromCache(ctx, cacheKey); err == nil {
		if invoice, ok := cached.(*InvoiceResponse); ok {
			return &utils.ApiResponse{
				StatusCode: http.StatusOK,
				Message:    "Invoice retrieved successfully",
				Data:       invoice,
			}
		}
	}
	var bName sql.NullString
	var sender_country string
	var first_name string
	var last_name string
	var email string

	const qq string = `SELECT business_name, country, email,
		first_name, last_name FROM users WHERE id = $1
		AND is_active = true
		AND email_verified = true
		AND first_name IS NOT NULL 
		AND first_name <> '' 
		AND last_name IS NOT NULL 
		AND last_name <> ''
	`
	err := is.postgres.QueryRow(ctx, qq, userId).Scan(&bName, &sender_country, &email, &first_name, &last_name)

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

	const query string = `
		WITH invoice_data AS (
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
				i.address_country,
				i.created_by_id,
				COALESCE(
					json_agg(
						json_build_object(
							'item_id', ii.id,
							'invoice_type', ii.item_type,
							'description', ii.description,
							'quantity', ii.quantity,
							'unit_price', ii.unit_price,
							'discount', ii.discount,
							'amount', ii.amount,
							'created_at', ii.created_at
						) ORDER BY ii.created_at
					) FILTER (WHERE ii.id IS NOT NULL), 
					'[]'::json
				) as items
			FROM invoice i
			LEFT JOIN invoice_items ii ON i.id = ii.invoice_id
			WHERE i.id = $1
			GROUP BY i.id
		)
		SELECT * FROM invoice_data
	`

	var result struct {
		ID             string          `db:"id"`
		InvoiceNumber  string          `db:"invoice_number"`
		InvoiceURL     string          `db:"invoice_url"`
		Title          sql.NullString  `db:"title"`
		PayerEmail     string          `db:"payer_email"`
		PayerName      sql.NullString  `db:"payer_name"`
		SubTotal       float64         `db:"sub_total"`
		ServiceFee     float64         `db:"service_fee"`
		Total          float64         `db:"total"`
		Currency       string          `db:"currency"`
		Status         string          `db:"status"`
		DueDate        time.Time       `db:"due_date"`
		PaidAt         sql.NullTime    `db:"paid_at"`
		CreatedAt      time.Time       `db:"created_at"`
		UpdatedAt      time.Time       `db:"updated_at"`
		AddressCountry sql.NullString  `db:"address_country"`
		CreatedByID    string          `db:"created_by_id"`
		Items          json.RawMessage `db:"items"`
	}

	err = is.postgres.QueryRow(ctx, query, invoiceId).Scan(
		&result.ID,
		&result.InvoiceNumber,
		&result.InvoiceURL,
		&result.Title,
		&result.PayerEmail,
		&result.PayerName,
		&result.SubTotal,
		&result.ServiceFee,
		&result.Total,
		&result.Currency,
		&result.Status,
		&result.DueDate,
		&result.PaidAt,
		&result.CreatedAt,
		&result.UpdatedAt,
		&result.AddressCountry,
		&result.CreatedByID,
		&result.Items,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return &utils.ApiResponse{
				StatusCode: http.StatusNotFound,
				Message:    "Invoice not found",
			}
		}
		log.Error("failed to fetch invoice", "error", err, "invoice_id", invoiceId)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to retrieve invoice",
		}
	}

	if result.CreatedByID != userId && role != string(user.RoleAdmin) {
		log.Warn("unauthorized invoice access attempt",
			"invoice_id", invoiceId,
			"owner_id", result.CreatedByID,
			"requester_id", userId,
		)
		return &utils.ApiResponse{
			StatusCode: http.StatusForbidden,
			Message:    "You don't have permission to view this invoice",
		}
	}

	var items []InvoiceItems
	if err := json.Unmarshal(result.Items, &items); err != nil {
		log.Error("failed to parse invoice items", "error", err)
		items = []InvoiceItems{}
	}

	sender := InvoiceSenderDetails{
		UserId:       userId,
		Name:         first_name + " " + last_name,
		Email:        email,
		Location:     sender_country,
		BusinessName: &bName.String,
	}

	invoice := InvoiceResponse{
		ID:            result.ID,
		InvoiceNumber: result.InvoiceNumber,
		InvoiceURL:    result.InvoiceURL,
		Title:         result.Title.String,
		PayerEmail:    result.PayerEmail,
		PayerName:     result.PayerName.String,
		SubTotal:      result.SubTotal,
		ServiceFee:    result.ServiceFee,
		Total:         result.Total,
		Currency:      result.Currency,
		Status:        result.Status,
		DueDate:       result.DueDate,
		CreatedAt:     result.CreatedAt,
		UpdatedAt:     result.UpdatedAt,
		Country:       result.AddressCountry.String,
		Items:         items,
		CreatedBy:     sender,
	}

	if result.PaidAt.Valid {
		invoice.PaidAt = &result.PaidAt.Time
	}

	go is.cacheInvoice(context.Background(), cacheKey, &invoice)

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "Invoice retrieved successfully",
		Data:       invoice,
	}
}

func (is *InvoiceService) GetInvoiceSearch(ctx context.Context, invoiceUrl, invoiceId, userId, role string) *utils.ApiResponse {
	log := is.log
	if invoiceId == "" && invoiceUrl == "" {
		return &utils.ApiResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "You must provide either invoice ID or URL",
		}
	}

	cacheKey := fmt.Sprintf("invoice_search:%s:%s", invoiceId, invoiceUrl)
	if cached, err := is.getFromCache(ctx, cacheKey); err == nil {
		if invoice, ok := cached.(*InvoiceResponse); ok {
			return &utils.ApiResponse{
				StatusCode: http.StatusOK,
				Message:    "Invoice retrieved successfully",
				Data:       invoice,
			}
		}
	}

	var bName sql.NullString
	var sender_country string
	var first_name string
	var last_name string
	var email string
	var phone_number sql.NullString

	const query string = `
	WITH invoice_data AS (
		SELECT
			i.id, i.invoice_number, i.invoice_url, i.title,
			i.payer_email, i.payer_name, i.sub_total, i.service_fee, i.total,
			i.currency, i.status, i.due_date, i.paid_at,
			i.created_at, i.updated_at, i.address_country, i.created_by_id,
			u.first_name, u.last_name, u.country, u.email, u.business_name, u.phone_number,
			json_agg(
				json_build_object(
					'invoice_type', ii.item_type,
					'description', ii.description,
					'quantity', ii.quantity,
					'unit_price', ii.unit_price,
					'discount', ii.discount,
					'amount', ii.amount
				) ORDER BY ii.created_at
			) FILTER (WHERE ii.id IS NOT NULL) as items
		FROM invoice i
		LEFT JOIN users u ON i.created_by_id = u.id
		LEFT JOIN invoice_items ii ON i.id = ii.invoice_id
		WHERE 
			(($1::UUID IS NOT NULL AND i.id = $1) OR 
			 ($2::TEXT IS NOT NULL AND i.invoice_url = $2))
		GROUP BY i.id, u.first_name, u.last_name, u.country, u.email, u.business_name, u.phone_number
	)
	SELECT * FROM invoice_data
	`
	var invoiceIdParam interface{}
	var invoiceUrlParam interface{}

	if invoiceId != "" {
		if _, err := uuid.Parse(invoiceId); err != nil {
			return &utils.ApiResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "Invalid invoice ID format",
			}
		}
		invoiceIdParam = invoiceId
	}
	if invoiceUrl != "" {
		invoiceUrlParam = invoiceUrl
	}

	var invoice struct {
		ID             string          `db:"id"`
		InvoiceNumber  string          `db:"invoice_number"`
		InvoiceURL     string          `db:"invoice_url"`
		Title          sql.NullString  `db:"title"`
		PayerEmail     string          `db:"payer_email"`
		PayerName      sql.NullString  `db:"payer_name"`
		SubTotal       float64         `db:"sub_total"`
		ServiceFee     float64         `db:"service_fee"`
		Total          float64         `db:"total"`
		Currency       string          `db:"currency"`
		Status         string          `db:"status"`
		DueDate        time.Time       `db:"due_date"`
		PaidAt         sql.NullTime    `db:"paid_at"`
		CreatedAt      time.Time       `db:"created_at"`
		UpdatedAt      time.Time       `db:"updated_at"`
		AddressCountry sql.NullString  `db:"address_country"`
		CreatedBy      string          `db:"created_by_id"`
		Items          json.RawMessage `db:"items"`
	}

	err := is.postgres.QueryRow(ctx, query, invoiceIdParam, invoiceUrlParam).Scan(
		&invoice.ID,
		&invoice.InvoiceNumber,
		&invoice.InvoiceURL,
		&invoice.Title,
		&invoice.PayerEmail,
		&invoice.PayerName,
		&invoice.SubTotal,
		&invoice.ServiceFee,
		&invoice.Total,
		&invoice.Currency,
		&invoice.Status,
		&invoice.DueDate,
		&invoice.PaidAt,
		&invoice.CreatedAt,
		&invoice.UpdatedAt,
		&invoice.AddressCountry,
		&invoice.CreatedBy,
		&first_name,
		&last_name,
		&sender_country,
		&email,
		&bName,
		&phone_number,
		&invoice.Items,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return &utils.ApiResponse{
				StatusCode: http.StatusNotFound,
				Message:    "Invoice not found",
			}
		}
		log.Error("failed to fetch invoice", "error", err, "invoice_id", invoiceId, "invoice_url", invoiceUrl)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to retrieve invoice",
		}
	}

	var items []InvoiceItems
	if invoice.Items != nil {
		if err := json.Unmarshal(invoice.Items, &items); err != nil {
			log.Error("failed to parse invoice items", "error", err)
			items = []InvoiceItems{}
		}
	}

	sender := InvoiceSenderDetails{
		UserId:       invoice.CreatedBy,
		Name:         first_name + " " + last_name,
		Email:        email,
		Location:     sender_country,
		BusinessName: &bName.String,
		PhoneNumber:  &phone_number.String,
	}

	response := InvoiceResponse{
		ID:            invoice.ID,
		InvoiceNumber: invoice.InvoiceNumber,
		InvoiceURL:    invoice.InvoiceURL,
		Title:         invoice.Title.String,
		PayerEmail:    invoice.PayerEmail,
		PayerName:     invoice.PayerName.String,
		SubTotal:      invoice.SubTotal,
		ServiceFee:    invoice.ServiceFee,
		Total:         invoice.Total,
		Currency:      invoice.Currency,
		Status:        invoice.Status,
		DueDate:       invoice.DueDate,
		CreatedAt:     invoice.CreatedAt,
		UpdatedAt:     invoice.UpdatedAt,
		Country:       invoice.AddressCountry.String,
		CreatedBy:     sender,
		Items:         items,
	}

	if invoice.PaidAt.Valid {
		response.PaidAt = &invoice.PaidAt.Time
	}

	go is.cacheInvoice(context.Background(), cacheKey, &response)

	if userId != invoice.CreatedBy && role != string(user.RoleAdmin) {
		go is.trackInvoiceView(context.Background(), invoice.ID)
	}

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "Invoice retrieved successfully",
		Data:       response,
	}
}

func (is *InvoiceService) canAccessInvoice(ownerID, userID, role string) bool {
	return ownerID == userID || role == string(user.RoleAdmin)
}

func (is *InvoiceService) getFromCache(ctx context.Context, key string) (interface{}, error) {
	data, err := is.redis.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	var invoice InvoiceResponse
	if err := json.Unmarshal([]byte(data), &invoice); err != nil {
		return nil, err
	}

	return &invoice, nil
}

func (is *InvoiceService) cacheInvoice(ctx context.Context, key string, invoice *InvoiceResponse) {
	data, err := json.Marshal(invoice)
	if err != nil {
		is.log.Error("failed to marshal invoice for cache", "error", err)
		return
	}

	if err := is.redis.Set(ctx, key, data, 90*time.Second).Err(); err != nil {
		is.log.Error("failed to cache invoice", "error", err)
	}
}

func (is *InvoiceService) trackInvoiceView(ctx context.Context, invoiceID string) {
	const updateQuery string = `
		UPDATE invoice 
		SET status = 'viewed', updated_at = NOW() 
		WHERE id = $1 AND status = 'sent'
	`
	is.postgres.Exec(ctx, updateQuery, invoiceID)
	is.DeleteFromRedisCache(ctx, invoiceID)
}

func (is *InvoiceService) DeleteInvoice(ctx context.Context, userId, invoiceId string) *utils.ApiResponse {
	const deleteQuery = `
		WITH deleted_invoice AS (
			DELETE FROM invoice 
			WHERE id = $1 
			AND created_by_id = $2 
			AND paid_at IS NULL
			RETURNING id
		)
		DELETE FROM invoice_items 
		WHERE invoice_id IN (SELECT id FROM deleted_invoice)
		RETURNING invoice_id`

	var deletedId string
	err := is.postgres.QueryRow(ctx, deleteQuery, invoiceId, userId).Scan(&deletedId)

	if err != nil {
		if err == pgx.ErrNoRows {
			var exists bool
			var isPaid bool
			var isOwner bool
			checkErr := is.postgres.QueryRow(ctx, `
				SELECT 
					EXISTS(SELECT 1 FROM invoice WHERE id = $1),
					EXISTS(SELECT 1 FROM invoice WHERE id = $1 AND paid_at IS NOT NULL),
					EXISTS(SELECT 1 FROM invoice WHERE id = $1 AND created_by_id = $2)
			`, invoiceId, userId).Scan(&exists, &isPaid, &isOwner)

			if checkErr != nil || !exists {
				return &utils.ApiResponse{
					StatusCode: http.StatusNotFound,
					Message:    "Invoice not found",
				}
			}
			if isPaid {
				return &utils.ApiResponse{
					StatusCode: http.StatusBadRequest,
					Message:    "Cannot delete paid invoice",
				}
			}
			if !isOwner {
				return &utils.ApiResponse{
					StatusCode: http.StatusForbidden,
					Message:    "Unauthorized to delete this invoice",
				}
			}
		}
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to delete invoice",
		}
	}
	is.DeleteFromRedisCache(ctx, invoiceId)

	return &utils.ApiResponse{
		Message:    "Successful",
		StatusCode: http.StatusOK,
	}
}

func (is *InvoiceService) EditInvoice(ctx context.Context, userId, invoiceId string, dto CreateInvoiceDTO) *utils.ApiResponse {
	tx, err := is.postgres.Begin(ctx)
	if err != nil {
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Server is currently unavailable, kindly contact support",
		}
	}
	defer tx.Rollback(ctx)

	const oq string = `SELECT status, created_by_id FROM invoice WHERE id = $1`
	var status string
	var createdById string
	err = tx.QueryRow(ctx, oq,
		invoiceId).Scan(&status, &createdById)

	if err != nil {
		if err == pgx.ErrNoRows {
			return &utils.ApiResponse{
				StatusCode: http.StatusNotFound,
				Message:    "Invoice not found",
			}
		}
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to fetch invoice",
		}
	}

	if createdById != userId {
		return &utils.ApiResponse{
			StatusCode: http.StatusForbidden,
			Message:    "Unauthorized to edit this invoice",
		}
	}

	if status != "draft" {
		return &utils.ApiResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "Can only edit draft invoices",
		}
	}

	subtotal, serviceFee, total, err := is.validateAndCalculateInvoice(dto)
	if err != nil {
		is.log.Error("error trying to calculate invoice numbers", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		}
	}

	const uis string = `UPDATE invoice SET 
			title = $1,
			payer_name = $2,
			payer_email = $3,
			address_country = $4,
			service_fee = $5,
			sub_total = $6,
			total = $7,
			due_date = $8,
			updated_at = NOW()
		WHERE id = $9`
	_, err = tx.Exec(ctx, uis,
		dto.Title, dto.RecipientName, dto.Email, dto.Country,
		serviceFee, subtotal, total, dto.DueDate, invoiceId)

	if err != nil {
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to update invoice",
		}
	}

	existingItemIds := make(map[string]bool)
	const ei string = `SELECT id FROM invoice_items WHERE invoice_id = $1`
	rows, err := tx.Query(ctx, ei, invoiceId)
	if err != nil {
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to fetch existing items",
		}
	}
	for rows.Next() {
		var id string
		rows.Scan(&id)
		existingItemIds[id] = true
	}
	rows.Close()

	itemsToKeep := make(map[string]bool)
	for _, item := range dto.InvoiceItems {
		if item.ItemId != "" {
			itemsToKeep[item.ItemId] = true
		}
	}

	for id := range existingItemIds {
		if !itemsToKeep[id] {
			const di string = `DELETE FROM invoice_items WHERE id = $1`
			_, err = tx.Exec(ctx, di, id)
			if err != nil {
				return &utils.ApiResponse{
					StatusCode: http.StatusInternalServerError,
					Message:    "Failed to delete removed items",
				}
			}
		}
	}

	for _, item := range dto.InvoiceItems {
		if item.ItemId != "" && existingItemIds[item.ItemId] {
			const ui string = `
				UPDATE invoice_items SET
					invoice_type = $1,
					description = $2,
					quantity = $3,
					unit_price = $4,
					discount = $5,
					amount = $6,
					updated_at = NOW()
				WHERE id = $7 AND invoice_id = $8
			`
			_, err = tx.Exec(ctx, ui,
				item.InvoiceType, item.Description, item.Quantity,
				item.UnitPrice, item.Discount, item.Amount,
				item.ItemId, invoiceId)
		} else {
			const ud string = `INSERT INTO invoice_items 
					(invoice_id, invoice_type, description, 
					quantity, unit_price, discount, amount)
					VALUES ($1, $2, $3, $4, $5, $6, $7)`
			_, err = tx.Exec(ctx, ud,
				invoiceId, item.InvoiceType, item.Description, item.Quantity,
				item.UnitPrice, item.Discount, item.Amount)
		}

		if err != nil {
			return &utils.ApiResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    "Failed to update invoice items",
			}
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to save changes",
		}
	}
	is.DeleteFromRedisCache(ctx, invoiceId)

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "Invoice updated successfully",
	}
}

func (is *InvoiceService) SendInvoice(ctx context.Context, userId, invoiceId, email string) *utils.ApiResponse {
	var (
		invoice_url string
		payer_name  string
		payer_email string
		first_name  string
		last_name   string
		status      string
	)

	const query string = `
		WITH invoice_update AS (
			UPDATE invoice 
			SET status = 'sent', updated_at = NOW()
			WHERE id = $1 
				AND created_by_id = $2 
				AND status IN ('draft', 'viewed')
			RETURNING invoice_url, payer_name, payer_email, status
		)
		SELECT 
			iu.invoice_url,
			iu.payer_name,
			iu.payer_email,
			u.first_name,
			u.last_name,
			iu.status
		FROM invoice_update iu
		LEFT JOIN users u ON u.id = $2`

	err := is.postgres.QueryRow(ctx, query, invoiceId, userId).Scan(
		&invoice_url, &payer_name, &payer_email, &first_name, &last_name, &status)

	if err != nil {
		if err == pgx.ErrNoRows {
			var exists bool
			var isOwner bool
			checkErr := is.postgres.QueryRow(ctx, `
				SELECT 
					EXISTS(SELECT 1 FROM invoice WHERE id = $1),
					EXISTS(SELECT 1 FROM invoice WHERE id = $1 AND created_by_id = $2)
			`, invoiceId, userId).Scan(&exists, &isOwner)

			if checkErr != nil || !exists {
				return &utils.ApiResponse{
					StatusCode: http.StatusNotFound,
					Message:    "Invoice not found",
				}
			}
			if !isOwner {
				return &utils.ApiResponse{
					StatusCode: http.StatusForbidden,
					Message:    "Access denied",
				}
			}
			return &utils.ApiResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "Invoice already sent or paid",
			}
		}

		is.log.Error("failed to send invoice", "error", err, "user_id", userId)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to process request",
		}
	}
	recipientEmail := email
	if recipientEmail == "" {
		recipientEmail = payer_email
	}

	if recipientEmail == "" {
		return &utils.ApiResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "No recipient email provided",
		}
	}

	go func() {
		senderName := strings.TrimSpace(fmt.Sprintf("%s %s", first_name, last_name))
		invoiceURL := fmt.Sprintf("https://usestellance.com/client/%s", url.QueryEscape(invoice_url))

		if err := is.mail.SendInvoiceUrlMail(recipientEmail, payer_name, senderName, invoiceURL); err != nil {
			is.log.Error("failed to send invoice email",
				"error", err,
				"invoice_id", invoiceId,
				"recipient", recipientEmail)
		} else {
			is.log.Info("invoice email sent successfully",
				"invoice_id", invoiceId,
				"recipient", recipientEmail)
		}
	}()

	is.DeleteFromRedisCache(ctx, invoiceId)
	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "Invoice sent successfully",
	}
}

func (is *InvoiceService) ReviewInvoice(ctx context.Context, invoiceId string, approve bool, userId, role string) *utils.ApiResponse {
	var (
		id    string
		query string
		cId   string
	)

	const ic string = `
			SELECT id, created_by_id FROM invoice WHERE id = $1 AND paid_at IS NULL
		`
	err := is.postgres.QueryRow(ctx, ic, invoiceId).Scan(&id, &cId)
	if err != nil {
		if err == pgx.ErrNoRows {
			return &utils.ApiResponse{
				StatusCode: http.StatusNotFound,
				Message:    "Invoice not found, or either has already been paid for. Thus, cannot be reviewed",
			}
		}
	}

	if userId != "" && role != "" {
		if userId == cId {
			return &utils.ApiResponse{
				StatusCode: http.StatusForbidden,
				Message:    "You cannot review your own invoices, kindly forward a copy to your client for approval",
			}
		}
	}

	if approve {
		query = `
			UPDATE invoice 
			SET approved = true, 
				approved_date = NOW(),
				status = 'viewed',
				updated_at = NOW()
			WHERE id = $1 
				AND approved = false 
				AND rejected IS NOT TRUE`
	} else {
		query = `
			UPDATE invoice 
			SET rejected = true, 
				rejected_date = NOW(),
				status = 'viewed',
				updated_at = NOW()
			WHERE id = $1 
				AND approved = false 
				AND rejected IS NOT TRUE`
	}

	result, err := is.postgres.Exec(ctx, query, invoiceId)
	if err != nil {
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to review invoice",
		}
	}

	if result.RowsAffected() == 0 {
		var exists, isApproved, isRejected bool
		err = is.postgres.QueryRow(ctx, `
			SELECT 
				EXISTS(SELECT 1 FROM invoice WHERE id = $1),
				COALESCE((SELECT approved FROM invoice WHERE id = $1), false),
				COALESCE((SELECT rejected FROM invoice WHERE id = $1), false)
		`, invoiceId).Scan(&exists, &isApproved, &isRejected)

		if err != nil || !exists {
			return &utils.ApiResponse{
				StatusCode: http.StatusNotFound,
				Message:    "Invoice not found",
			}
		}
		if isApproved || isRejected {
			return &utils.ApiResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "Invoice has already been reviewed",
			}
		}
	}

	action := "approved"
	if !approve {
		action = "rejected"
	}
	is.DeleteFromRedisCache(ctx, invoiceId)

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    fmt.Sprintf("Invoice %s successfully", action),
	}
}

func (is *InvoiceService) UpdateOverdueInvoices(ctx context.Context) error {
	const query = `
		UPDATE invoice 
		SET 
			status = 'overdue',
			updated_at = NOW()
		WHERE 
			status IN ('sent', 'viewed', 'approved', 'pending')
			AND due_date < CURRENT_DATE
			AND paid_at IS NULL
		RETURNING id`

	rows, err := is.postgres.Query(ctx, query)
	if err != nil {
		is.log.Error("Failed to update overdue invoices", "error", err)
		return err
	}
	defer rows.Close()

	var updatedIDs []string

	for rows.Next() {
		var invoiceID string
		if err := rows.Scan(&invoiceID); err != nil {
			is.log.Error("Failed to scan invoice ID", "error", err)
			continue
		}
		updatedIDs = append(updatedIDs, invoiceID)
		is.DeleteFromRedisCache(ctx, invoiceID)
	}

	if err := rows.Err(); err != nil {
		is.log.Error("Row iteration error", "error", err)
		return err
	}

	is.log.Info("Updated overdue invoices", "count", len(updatedIDs))
	return nil
}

func (is *InvoiceService) DeleteFromRedisCache(ctx context.Context, invoiceId string) error {
	err := is.redis.Del(ctx, fmt.Sprintf("invoice:%s", invoiceId)).Err()
	if err != nil {
		is.log.Info("Failed to delete Redis key", "error", err)
		return err
	}
	return nil
}
