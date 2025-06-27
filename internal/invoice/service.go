package invoice

import (
	"context"
	"database/sql"
	"encoding/json"
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
func (is *InvoiceService) GetInvoiceSearch(ctx context.Context, invoiceUrl, invoiceId, userId, role string) *utils.ApiResponse {
	log := is.log
	if invoiceId == "" && invoiceUrl == "" {
		return &utils.ApiResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "You must provide either invoice ID or URL",
		}
	}

	cacheKey := fmt.Sprintf("invoice:%s:%s", invoiceId, invoiceUrl)
	if cached, err := is.getFromCache(ctx, cacheKey); err == nil {
		if invoice, ok := cached.(*InvoiceResponse); ok {
			return &utils.ApiResponse{
				StatusCode: http.StatusOK,
				Message:    "Invoice retrieved successfully",
				Data:       invoice,
			}
		}
	}

	const query string = `
		WITH invoice_data AS (
			SELECT
				i.id, i.invoice_number, i.invoice_url, i.title,
				i.payer_email, i.payer_name, i.sub_total, i.service_fee, i.total,
				i.currency, i.status, i.due_date, i.paid_at,
				i.created_at, i.updated_at, i.address_country, i.created_by_id,
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
			LEFT JOIN invoice_items ii ON i.id = ii.invoice_id
			WHERE 
				(($1::UUID IS NOT NULL AND i.id = $1) OR 
				 ($2::TEXT IS NOT NULL AND i.invoice_url = $2))
			GROUP BY i.id
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

	// if !is.canAccessInvoice(invoice.CreatedBy, userId, role) {
	// 	return &utils.ApiResponse{
	// 		StatusCode: http.StatusForbidden,
	// 		Message:    "You don't have permission to view this invoice",
	// 	}
	// }

	var items []InvoiceItems
	if invoice.Items != nil {
		if err := json.Unmarshal(invoice.Items, &items); err != nil {
			log.Error("failed to parse invoice items", "error", err)
			items = []InvoiceItems{}
		}
	}

	fmt.Println(items, "the i items")

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
		CreatedBy:     &invoice.CreatedBy,
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

	if err := is.redis.Set(ctx, key, data, 10*time.Minute).Err(); err != nil {
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
}
