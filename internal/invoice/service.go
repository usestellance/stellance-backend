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
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/The-True-Hooha/stellance-backend/internal/logo"
	"github.com/The-True-Hooha/stellance-backend/internal/notifications"
	"github.com/The-True-Hooha/stellance-backend/internal/user"
	"github.com/The-True-Hooha/stellance-backend/mail"
	"github.com/The-True-Hooha/stellance-backend/pkg/config"
	jwt_ "github.com/The-True-Hooha/stellance-backend/pkg/jwt"
	"github.com/The-True-Hooha/stellance-backend/pkg/utils"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const (
	SERVICE_FEE_PERCENTAGE = 2.5
)

type InvoiceService struct {
	log         *slog.Logger
	postgres    *pgxpool.Pool
	redis       *redis.Client
	jwt         *jwt_.JwtTokenServiceConfig
	mail        *mail.Mailer
	logoService *logo.LogoService
}

func NewInvoiceService() *InvoiceService {
	return &InvoiceService{
		log:         config.GetAppContainer().Log,
		postgres:    config.GetAppContainer().Postgres,
		redis:       config.GetAppContainer().Redis,
		jwt:         jwt_.JwtTokenService(),
		mail:        mail.NewMailer(),
		logoService: logo.NewLogoService(),
	}
}

func (is *InvoiceService) GenerateNewInvoice(ctx context.Context, dto CreateInvoiceDTO, userId string, logoFile *logo.LogoFileData) *utils.ApiResponse {
	subtotal, serviceFee, total, err := is.validateAndCalculateInvoice(dto)
	if err != nil {
		is.log.Error("error trying to calculate invoice numbers", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		}
	}

	var logoID sql.NullString
	var logoURL sql.NullString

	tx, err := is.postgres.Begin(ctx)
	if err != nil {
		is.log.Error("failed to begin transaction", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to process request. Please try again.",
		}
	}
	defer tx.Rollback(ctx)

	if logoFile != nil {
		createLogo, err := is.logoService.UploadAndCreateLogo(ctx, tx, userId, logoFile)
		if err != nil {
			is.log.Error("failed to upload and create new invoice", "error", err)
			return &utils.ApiResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    "failed to create new invoice, try again ",
			}
		}

		logoID = sql.NullString{String: createLogo.LogoID, Valid: true}
		logoURL = sql.NullString{String: createLogo.LogoUrl, Valid: true}
	} else {
		defaultLogo, err := is.logoService.GetDefaultLogoByUserID(ctx, userId)
		if err == nil && defaultLogo != nil {
			logoURL = sql.NullString{String: defaultLogo.LogoPresignedURL, Valid: true}
			logoID = sql.NullString{String: defaultLogo.ID, Valid: true}
		} else if err != nil && err != pgx.ErrNoRows {
			is.log.Error("failed to fetch default logo from database", "error", err)
			return &utils.ApiResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    "failed to create new invoice",
			}
		}
		// If no default logo (ErrNoRows or nil), leave logoID and logoURL as null
	}

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
	sub_total, service_fee, total, currency, title, status,due_date, address_country, payer_name, template_id, logo_id, notes)
	VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)RETURNING id, created_at`

	var invoiceId string
	var createdAt time.Time

	err = tx.QueryRow(ctx, invoiceQ, invoiceNumber, invoice_url, userId, dto.Email, subtotal, serviceFee, total, utils.USDC, dto.Title, InvoiceStatusDraft, dueDate, dto.Country, dto.RecipientName, dto.TemplateID, logoID, dto.Note).Scan(&invoiceId, &createdAt)
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
	f := false

	logoURLStr := ""
	if logoURL.Valid {
		logoURLStr = logoURL.String
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
		Approved:      &f,
		ReviewDate:    nil,
		TemplateID:    dto.TemplateID,
		LogoURL:       logoURLStr,
		Note:          dto.Note,
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
	dateStr := time.Now().Format("20060102")

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
	return fmt.Sprintf("INV-%s-%04d", dateStr, invoiceNumber), nil
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
			InvoiceStatusViewed:    true,
			InvoiceStatusPaid:      true,
			InvoiceStatusOverdue:   true,
			InvoiceStatusCancelled: true,
			InvoiceStatusRefunded:  true,
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

	if dto.Search != "" {
		searchTerm := "%" + strings.TrimSpace(dto.Search) + "%"
		argCount++
		whereClause += fmt.Sprintf(` AND (
		i.invoice_number ILIKE $%d OR
		i.payer_email ILIKE $%d OR
		i.title ILIKE $%d OR
		i.payer_name ILIKE $%d OR
		i.template_id ILIKE $%d
	)`, argCount, argCount, argCount, argCount, argCount)
		countArgs = append(countArgs, searchTerm)
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
		var (
			invoice            InvoiceResponse
			payerWalletAddress sql.NullString
			title              sql.NullString
			paidAt             sql.NullTime
			country            sql.NullString
			logoID             sql.NullString
			notes              sql.NullString
			approved           bool
			approvedDate       sql.NullTime
			rejectedDate       sql.NullTime
			reviewDate         sql.NullTime
			relevance          float64
		)

		scanDest := []interface{}{
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
			&approved,
			&approvedDate,
			&rejectedDate,
			&logoID,
			&notes,
			&invoice.TemplateID,
		}

		if dto.Search != "" {
			scanDest = append(scanDest, &relevance)
		}

		err := rows.Scan(
			scanDest...,
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

		if approved && approvedDate.Valid {
			reviewDate = approvedDate
		} else {
			reviewDate = rejectedDate
		}

		logoURL := ""
		if logoID.Valid {
			url, err := is.logoService.GetSignedDownloadURL(ctx, logoID.String)
			if err != nil {
				is.log.Warn("failed to get logo URL", "error", err, "logo_id", logoID.String)
			} else {
				logoURL = url
			}
		}

		sender := InvoiceSenderDetails{
			UserId:       user_id,
			Name:         first_name + " " + last_name,
			Email:        email,
			Location:     sender_country,
			BusinessName: &bName.String,
		}
		invoice.CreatedBy = sender
		invoice.LogoURL = logoURL
		if notes.Valid {
			invoice.Note = notes.String
		}
		invoice.ReviewDate = &reviewDate.Time
		invoice.Approved = &approved
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
			PageCount:         len(invoices),
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

// func (is *InvoiceService) buildInvoiceQuery(filters InvoiceFiltersDto, userId string) (string, []interface{}) {
// 	offset := (filters.Page - 1) * filters.Count

// 	if filters.Search != "" {

// 	}

// 	query := `
// 		SELECT
// 			i.id,
// 			i.invoice_number,
// 			i.invoice_url,
// 			i.title,
// 			i.payer_email,
// 			i.payer_name,
// 			i.payer_wallet_address,
// 			i.sub_total,
// 			i.service_fee,
// 			i.total,
// 			i.currency,
// 			i.status,
// 			i.due_date,
// 			i.paid_at,
// 			i.created_at,
// 			i.updated_at,
// 			i.address_country,
// 			i.approved,
// 			i.approved_date,
// 			i.rejected_date,
// 			i.logo_id,
// 			i.notes,
// 			i.template_id
// 		FROM invoice i
// 		WHERE i.created_by_id = $1
// 	`
// 	args := []interface{}{userId}
// 	argCount := 1

// 	if filters.Status != "" {
// 		argCount++
// 		query += fmt.Sprintf(" AND i.status = $%d", argCount)
// 		args = append(args, filters.Status)
// 		if filters.Status == "overdue" {
// 			query += " AND i.due_date < CURRENT_DATE AND i.status != 'paid'"
// 		}
// 	}

// 	query += fmt.Sprintf(" ORDER BY i.created_at %s", filters.OrderBy)

// 	argCount++
// 	query += fmt.Sprintf(" LIMIT $%d", argCount)
// 	args = append(args, filters.Count)

// 	argCount++
// 	query += fmt.Sprintf(" OFFSET $%d", argCount)
// 	args = append(args, offset)

// 	return query, args
// }

func (is *InvoiceService) buildInvoiceQuery(filters InvoiceFiltersDto, userId string) (string, []interface{}) {
	offset := (filters.Page - 1) * filters.Count

	if filters.Search != "" {
		return is.buildSearchQuery(filters, userId, offset)
	}

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
			i.address_country,
			i.approved,
			i.approved_date,
			i.rejected_date,
			i.logo_id,
			i.notes,
			i.template_id
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

	orderDir := "DESC"
	if filters.OrderBy == utils.OrderByASC {
		orderDir = "ASC"
	}
	query += " ORDER BY i.created_at " + orderDir

	argCount++
	query += fmt.Sprintf(" LIMIT $%d", argCount)
	args = append(args, filters.Count)

	argCount++
	query += fmt.Sprintf(" OFFSET $%d", argCount)
	args = append(args, offset)

	return query, args
}

func (is *InvoiceService) buildSearchQuery(filters InvoiceFiltersDto, userId string, offset int) (string, []interface{}) {
	searchTerm := "%" + strings.TrimSpace(filters.Search) + "%"

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
			i.address_country,
			i.approved,
			i.approved_date,
			i.rejected_date,
			i.logo_id,
			i.notes,
			i.template_id,
			GREATEST(
				similarity(COALESCE(i.invoice_number, ''), $2),
				similarity(COALESCE(i.payer_email, ''), $2),
				similarity(COALESCE(i.title, ''), $2),
				similarity(COALESCE(i.payer_name, ''), $2),
				similarity(COALESCE(i.template_id, ''), $2)
			) AS relevance
		FROM invoice i
		WHERE i.created_by_id = $1
			AND (
				i.invoice_number ILIKE $3 OR
				i.payer_email ILIKE $3 OR
				i.title ILIKE $3 OR
				i.payer_name ILIKE $3 OR
				i.template_id ILIKE $3
			)
	`

	args := []interface{}{userId, strings.TrimSpace(filters.Search), searchTerm}
	argCount := 3

	if filters.Status != "" {
		argCount++
		query += fmt.Sprintf(" AND i.status = $%d", argCount)
		args = append(args, filters.Status)
		if filters.Status == "overdue" {
			query += " AND i.due_date < CURRENT_DATE AND i.status != 'paid'"
		}
	}

	query += " ORDER BY relevance DESC, i.created_at DESC"

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
	var logoID sql.NullString
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
				i.approved,
				i.approved_date,
				i.rejected_date,
				i.logo_id,
				i.notes,
				i.template_id,
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
		Notes          sql.NullString  `db:"notes"`
		SubTotal       float64         `db:"sub_total"`
		ServiceFee     float64         `db:"service_fee"`
		Total          float64         `db:"total"`
		Currency       string          `db:"currency"`
		TemplateID     string          `db:"template_id"`
		Status         string          `db:"status"`
		DueDate        time.Time       `db:"due_date"`
		PaidAt         sql.NullTime    `db:"paid_at"`
		CreatedAt      time.Time       `db:"created_at"`
		UpdatedAt      time.Time       `db:"updated_at"`
		AddressCountry sql.NullString  `db:"address_country"`
		CreatedByID    string          `db:"created_by_id"`
		Items          json.RawMessage `db:"items"`
		Approved       bool            `db:"approved"`
		ApprovedDate   sql.NullTime    `db:"approved_date"`
		RejectedDate   sql.NullTime    `db:"rejected_date"`
		ReviewDate     sql.NullTime
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
		&result.Approved,
		&result.ApprovedDate,
		&result.RejectedDate,
		&logoID,
		&result.Notes,
		&result.TemplateID,
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

	logoURL := ""
	if logoID.Valid {
		url, err := is.logoService.GetSignedDownloadURL(ctx, logoID.String)
		if err != nil {
			log.Warn("failed to get logo URL", "error", err, "logo_id", logoID.String)
		} else {
			logoURL = url
		}
	}

	if result.Approved && result.ApprovedDate.Valid {
		result.ReviewDate = result.ApprovedDate
	} else {
		result.ReviewDate = result.RejectedDate
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
		Approved:      &result.Approved,
		LogoURL:       logoURL,
		TemplateID:    TemplateIDType(result.TemplateID),
		Note:          result.Notes.String,
	}

	if result.PaidAt.Valid {
		invoice.PaidAt = &result.PaidAt.Time
	}

	if result.ReviewDate.Valid {
		invoice.ReviewDate = &result.ReviewDate.Time
	} else {
		invoice.ReviewDate = nil
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
			i.approved, i.approved_date, i.rejected_date, i.logo_id, template_id, i.notes,
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

	var reviewDate sql.NullTime

	var invoice struct {
		ID             string          `db:"id"`
		InvoiceNumber  string          `db:"invoice_number"`
		InvoiceURL     string          `db:"invoice_url"`
		Title          sql.NullString  `db:"title"`
		PayerEmail     string          `db:"payer_email"`
		TemplateID     string          `db:"template_id"`
		PayerName      sql.NullString  `db:"payer_name"`
		LogoID         sql.NullString  `db:"logo_id"`
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
		Notes          sql.NullString  `db:"note"`
		CreatedBy      string          `db:"created_by_id"`
		Items          json.RawMessage `db:"items"`
		Approved       bool            `db:"approved"`
		ApprovedDate   sql.NullTime    `db:"approved_date"`
		RejectedDate   sql.NullTime    `db:"rejected_date"`
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
		&invoice.Approved,
		&invoice.ApprovedDate,
		&invoice.RejectedDate,
		&invoice.LogoID,
		&invoice.TemplateID,
		&invoice.Notes,
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

	var walletAddress sql.NullString
	const w = `SELECT address from wallets WHERE user_id = $1 AND is_primary = true AND is_active = true`
	err = is.postgres.QueryRow(ctx, w, invoice.CreatedBy).Scan(&walletAddress)
	if err != nil {
		if err == pgx.ErrNoRows {
			is.log.Debug("no wallet details found for user", "userId", invoice.CreatedBy)
		}
		is.log.Error("failed to fetch wallet", "error", err, "userId", invoice.CreatedBy)
	}

	sender := InvoiceSenderDetails{
		UserId:         invoice.CreatedBy,
		Name:           first_name + " " + last_name,
		Email:          email,
		Location:       sender_country,
		BusinessName:   &bName.String,
		Wallet_address: &walletAddress.String,
	}

	if phone_number.Valid {
		sender.PhoneNumber = &phone_number.String
	}
	if bName.Valid {
		sender.BusinessName = &bName.String
	}

	if invoice.Approved && invoice.ApprovedDate.Valid {
		reviewDate = invoice.ApprovedDate
	} else {
		reviewDate = invoice.RejectedDate
	}

	logoURL := ""
	if invoice.LogoID.Valid {

		url, err := is.logoService.GetSignedDownloadURL(ctx, invoice.LogoID.String)
		if err != nil {
			log.Warn("failed to get logo URL", "error", err, "logo_id", invoice.LogoID.String)
		} else {
			logoURL = url
		}
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
		Approved:      &invoice.Approved,
		LogoURL:       logoURL,
		TemplateID:    TemplateIDType(invoice.TemplateID),
		Note:          invoice.Notes.String,
	}

	if invoice.PaidAt.Valid {
		response.PaidAt = &invoice.PaidAt.Time
	}

	if reviewDate.Valid {
		response.ReviewDate = &reviewDate.Time
	} else {
		response.ReviewDate = nil
	}

	go is.cacheInvoice(context.Background(), cacheKey, &response)

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "Invoice retrieved successfully",
		Data:       response,
	}
}

func (is *InvoiceService) GetInvoiceBySearchOnUser(
	ctx context.Context,
	search string,
	userId string,
	limit int,
	offset int,
) *utils.ApiResponse {

	log := is.log

	const defaultLimit = 10
	const maxLimit = 50

	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if offset < 0 {
		offset = 0
	}

	searchTerm := "%" + strings.TrimSpace(search) + "%"
	cacheKey := fmt.Sprintf("invoice-search:%s:%s:%d:%d", userId, searchTerm, limit, offset)

	if cached, err := is.getFromCache(ctx, cacheKey); err == nil {
		return &utils.ApiResponse{
			StatusCode: http.StatusOK,
			Message:    "Invoices retrieved successfully",
			Data:       cached,
		}
	}

	var total int
	countQuery := `
		SELECT COUNT(DISTINCT i.id)
		FROM invoice i
		WHERE
			i.created_by_id = $1
			AND (
				i.invoice_number ILIKE $2 OR
				i.payer_email   ILIKE $2 OR
				i.title         ILIKE $2 OR
				i.payer_name    ILIKE $2 OR
				i.template_id   ILIKE $2
			);
	`

	if err := is.postgres.QueryRow(ctx, countQuery, userId, searchTerm).Scan(&total); err != nil {
		log.Error("failed to count invoices", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to search invoices",
		}
	}

	const searchInvoiceQuery = `
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
				i.approved, 
				i.approved_date, 
				i.rejected_date, 
				i.logo_id, 
				i.template_id, 
				i.notes,

				u.first_name,
				u.last_name,
				u.country AS user_country,
				u.email,
				u.business_name,
				u.phone_number,
				json_agg(
					json_build_object(
						'invoice_type', ii.item_type,
						'description', ii.description,
						'quantity', ii.quantity,
						'unit_price', ii.unit_price,
						'discount', ii.discount,
						'amount', ii.amount
					)
					ORDER BY ii.created_at
				) FILTER (WHERE ii.id IS NOT NULL) AS items,
				GREATEST(
					similarity(i.invoice_number, $2),
					similarity(i.payer_email,   $2),
					similarity(i.title,         $2),
					similarity(i.payer_name,    $2),
					similarity(i.template_id,   $2)
				) AS relevance
			FROM invoice i
			LEFT JOIN users u ON i.created_by_id = u.id
			LEFT JOIN invoice_items ii ON i.id = ii.invoice_id
			WHERE
				i.created_by_id = $1
				AND (
					i.invoice_number ILIKE $2 OR
					i.payer_email   ILIKE $2 OR
					i.title         ILIKE $2 OR
					i.payer_name    ILIKE $2 OR
					i.template_id   ILIKE $2
				)
			GROUP BY i.id, u.id
		)
		SELECT *
		FROM invoice_data
		ORDER BY relevance DESC, created_at DESC
		LIMIT $3 OFFSET $4;
	`

	rows, err := is.postgres.Query(ctx, searchInvoiceQuery, userId, searchTerm, limit, offset)
	if err != nil {
		log.Error("failed to execute invoice search", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to search invoices",
		}
	}
	defer rows.Close()

	var responses []InvoiceResponse

	for rows.Next() {
		var inv struct {
			ID            string
			InvoiceNumber string
			InvoiceURL    string
			Title         sql.NullString
			PayerEmail    string
			PayerName     sql.NullString
			SubTotal      float64
			ServiceFee    float64
			Total         float64
			Currency      string
			Status        string
			DueDate       time.Time
			PaidAt        sql.NullTime
			CreatedAt     time.Time
			UpdatedAt     time.Time
			Country       sql.NullString
			CreatedBy     string
			Note          sql.NullString
			Approved      bool
			ApprovedDate  sql.NullTime
			RejectedDate  sql.NullTime
			LogoID        sql.NullString
			Items         json.RawMessage
			FirstName     string
			LastName      string
			UserCountry   string
			Email         string
			TemplateID    string
			Business      sql.NullString
			Phone         sql.NullString
			Relevance     float64
		}
		err := rows.Scan(&inv.ID, &inv.InvoiceNumber, &inv.InvoiceURL, &inv.Title, &inv.PayerEmail, &inv.PayerName, &inv.SubTotal, &inv.ServiceFee, &inv.Total, &inv.Currency, &inv.Status, &inv.DueDate, &inv.PaidAt, &inv.CreatedAt, &inv.UpdatedAt, &inv.Country, &inv.CreatedBy, &inv.Approved, &inv.ApprovedDate, &inv.RejectedDate, &inv.LogoID, &inv.TemplateID, &inv.Note, &inv.FirstName, &inv.LastName, &inv.UserCountry, &inv.Email, &inv.Business, &inv.Phone, &inv.Items, &inv.Relevance)
		if err != nil {
			log.Error("failed to scan invoice row", "error", err)
			continue
		}

		var items []InvoiceItems
		_ = json.Unmarshal(inv.Items, &items)

		logoURL := ""
		if inv.LogoID.Valid {
			url, err := is.logoService.GetSignedDownloadURL(ctx, inv.LogoID.String)
			if err != nil {
				is.log.Warn("failed to get logo URL", "error", err, "logo_id", inv.LogoID.String)
			} else {
				logoURL = url
			}
		}
		response := InvoiceResponse{
			ID:            inv.ID,
			InvoiceNumber: inv.InvoiceNumber,
			InvoiceURL:    inv.InvoiceURL,
			Title:         inv.Title.String,
			PayerEmail:    inv.PayerEmail,
			PayerName:     inv.PayerName.String,
			SubTotal:      inv.SubTotal,
			ServiceFee:    inv.ServiceFee,
			Total:         inv.Total,
			Currency:      inv.Currency,
			LogoURL:       logoURL,
			Status:        inv.Status,
			Note:          inv.Note.String,
			DueDate:       inv.DueDate,
			CreatedAt:     inv.CreatedAt,
			UpdatedAt:     inv.UpdatedAt,
			Country:       inv.Country.String,
			TemplateID:    TemplateIDType(inv.TemplateID),
			Items:         items,
			Approved:      &inv.Approved,
		}
		responses = append(responses, response)
	}

	meta := SearchPaginationMeta{
		Total:   total,
		Limit:   limit,
		Offset:  offset,
		HasNext: offset+limit < total,
		HasPrev: offset > 0,
	}

	payload := struct {
		Invoice []InvoiceResponse    `json:"invoice"`
		Meta    SearchPaginationMeta `json:"meta"`
	}{
		Invoice: responses,
		Meta:    meta,
	}

	go is.cacheInvoice(context.Background(), cacheKey, payload)

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "Invoices retrieved successfully",
		Data:       payload,
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

func (is *InvoiceService) cacheInvoice(
	ctx context.Context,
	key string,
	value any,
) {
	data, err := json.Marshal(value)
	if err != nil {
		is.log.Error("failed to marshal cache value", "error", err, "key", key)
		return
	}

	if err := is.redis.Set(ctx, key, data, 90*time.Second).Err(); err != nil {
		is.log.Error("failed to set cache value", "error", err, "key", key)
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

func (is *InvoiceService) SendInvoice(ctx context.Context, userId, invoiceId string, emails []string) *utils.ApiResponse {
	if len(emails) == 0 {
		return &utils.ApiResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "At least one recipient email is required",
		}
	}

	for _, email := range emails {
		if email == "" || !isValidEmail(email) {
			return &utils.ApiResponse{
				StatusCode: http.StatusBadRequest,
				Message:    fmt.Sprintf("Invalid email address: %s", email),
			}
		}
	}

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
			return is.handleInvoiceNotFound(ctx, invoiceId, userId)
		}

		is.log.Error("failed to send invoice", "error", err, "user_id", userId)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to process request",
		}
	}

	secret := os.Getenv("INVOICE_ACCESS_SECRET")
	if secret == "" {
		is.log.Error("INVOICE_ACCESS_SECRET not set")
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Server configuration error",
		}
	}

	allRecipients := append([]string{}, emails...)
	if payer_email != "" && !contains(emails, payer_email) {
		allRecipients = append(allRecipients, payer_email)
	}
	allRecipients = removeDuplicates(allRecipients)

	type EmailData struct {
		Recipient string
		Name      string
		URL       string
	}

	emailsToSend := make([]EmailData, 0, len(allRecipients))

	for _, recipient := range allRecipients {
		name := is.GetNameFromEmail(recipient)

		token, err := utils.GenerateInvoiceAccessToken(invoice_url, recipient, name, secret)
		if err != nil {
			is.log.Error("failed to generate access token",
				"error", err,
				"recipient", recipient)
			return &utils.ApiResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    "Failed to generate secure access tokens",
			}
		}

		invoiceURL := fmt.Sprintf("https://usestellance.com/client/%s?token=%s&name=%s",
			url.QueryEscape(invoice_url),
			url.QueryEscape(token),
			url.QueryEscape(name))

		emailsToSend = append(emailsToSend, EmailData{
			Recipient: recipient,
			Name:      name,
			URL:       invoiceURL,
		})
	}

	go func() {
		senderName := strings.TrimSpace(fmt.Sprintf("%s %s", first_name, last_name))
		if senderName == "" {
			senderName = "Stellance User"
		}

		successCount := 0
		failedEmails := []string{}

		for _, emailData := range emailsToSend {
			if err := is.mail.SendInvoiceUrlMail(mail.SendInvoiceEmailData{
				PrimaryRecipient: emailData.Recipient,
				PayerName:        payer_name,
				SenderName:       senderName,
				InvoiceURL:       emailData.URL,
			}); err != nil {
				is.log.Error("failed to send invoice email",
					"error", err,
					"invoice_id", invoiceId,
					"recipient", emailData.Recipient)
				failedEmails = append(failedEmails, emailData.Recipient)
			} else {
				successCount++
			}
		}

		is.log.Info("invoice email campaign completed",
			"invoice_id", invoiceId,
			"total_sent", successCount,
			"total_failed", len(failedEmails),
			"failed_emails", failedEmails,
			"total_recipients", len(emailsToSend))
	}()

	is.DeleteFromRedisCache(ctx, invoiceId)

	responseMessage := fmt.Sprintf("Invoice is being sent to %d recipient(s)", len(allRecipients))

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    responseMessage,
		Data: map[string]interface{}{
			"total_recipients": len(allRecipients),
			"invoice_url":      invoice_url,
		},
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, item) {
			return true
		}
	}
	return false
}

func removeDuplicates(slice []string) []string {
	seen := make(map[string]bool)
	result := []string{}

	for _, item := range slice {
		lowerItem := strings.ToLower(strings.TrimSpace(item))
		if lowerItem != "" && !seen[lowerItem] {
			seen[lowerItem] = true
			result = append(result, item)
		}
	}

	return result
}

func isValidEmail(email string) bool {
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	return emailRegex.MatchString(email)
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
				status = 'cancelled',
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
	var notifBody string
	if !approve {
		action = "rejected"
		notifBody = fmt.Sprintf("Your invoice with ID %s has been rejected at %s", invoiceId, time.Now().Format("02/01/2006 03:04PM"))
	} else {
		notifBody = fmt.Sprintf("Your invoice with ID %s has been approved at %s", invoiceId, time.Now().Format("02/01/2006 03:04PM"))
	}
	is.DeleteFromRedisCache(ctx, invoiceId)

	go func() {
		bgCtx := context.Background()

		notifData := notifications.CreateNotificationDto{
			Title:  "Invoice Review Update",
			UserId: cId,
			Body:   notifBody,
		}
		notifications.NewNotificationService().CreateNewNotification(bgCtx, notifData)

		var creatorEmail, creatorFirstName, creatorLastName, invoiceNumber, payerEmail, currency string
		var total float64
		err := is.postgres.QueryRow(bgCtx, `
			SELECT u.email, u.first_name, u.last_name, i.invoice_number, i.payer_email, i.total, i.currency
			FROM invoice i
			JOIN users u ON u.id = i.created_by_id
			WHERE i.id = $1
		`, invoiceId).Scan(&creatorEmail, &creatorFirstName, &creatorLastName, &invoiceNumber, &payerEmail, &total, &currency)
		if err != nil {
			is.log.Error("failed to fetch invoice details for review notification email", "error", err)
			return
		}

		creatorName := strings.TrimSpace(fmt.Sprintf("%s %s", creatorFirstName, creatorLastName))
		if creatorName == "" {
			creatorName = creatorEmail
		}

		dashboardURL := os.Getenv("FRONTEND_URL")
		if dashboardURL == "" {
			dashboardURL = "https://app.usestellance.com/dashboard"
		}

		emailData := mail.InvoiceReviewNotificationData{
			CreatorEmail:  creatorEmail,
			CreatorName:   creatorName,
			PayerName:     payerEmail,
			InvoiceNumber: invoiceNumber,
			Total:         fmt.Sprintf("%.2f", total),
			Currency:      strings.ToUpper(currency),
			Approved:      approve,
			DashboardURL:  dashboardURL,
		}
		if err := is.mail.SendInvoiceReviewNotification(emailData); err != nil {
			is.log.Error("failed to send invoice review notification email", "error", err, "creator_email", creatorEmail)
		}

	}()

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    fmt.Sprintf("Invoice %s successfully", action),
	}
}

func (is *InvoiceService) UpdateOverdueInvoices(ctx context.Context) error {
	is.log.Info("Starting invoice status update")

	var draftCount, rejectedCount, notRejectedCount int

	is.postgres.QueryRow(ctx, `SELECT COUNT(*) FROM invoice WHERE status = 'draft'`).Scan(&draftCount)
	is.postgres.QueryRow(ctx, `SELECT COUNT(*) FROM invoice WHERE status = 'draft' AND approved = false`).Scan(&rejectedCount)
	is.postgres.QueryRow(ctx, `SELECT COUNT(*) FROM invoice WHERE status = 'draft' AND (approved IS NULL OR approved = true)`).Scan(&notRejectedCount)

	is.log.Info("Draft invoice breakdown",
		"total_draft", draftCount,
		"rejected", rejectedCount,
		"not_rejected", notRejectedCount,
	)

	const cancelledQuery = `
		UPDATE invoice 
		SET 
			status = 'cancelled',
			updated_at = NOW()
		WHERE 
			status IN ('draft', 'sent', 'viewed', 'pending')
			AND approved = false
		RETURNING id`

	cancelledRows, err := is.postgres.Query(ctx, cancelledQuery)
	if err != nil {
		is.log.Error("Failed to update rejected invoices to cancelled", "error", err)
		return err
	}

	var cancelledIDs []string
	for cancelledRows.Next() {
		var invoiceID string
		if err := cancelledRows.Scan(&invoiceID); err != nil {
			is.log.Error("Failed to scan cancelled invoice ID", "error", err)
			continue
		}
		cancelledIDs = append(cancelledIDs, invoiceID)
		is.DeleteFromRedisCache(ctx, invoiceID)
	}
	cancelledRows.Close()

	const overdueQuery = `
		UPDATE invoice 
		SET 
			status = 'overdue',
			updated_at = NOW()
		WHERE 
			status IN ('draft', 'sent', 'viewed', 'pending')
			AND due_date < CURRENT_DATE
			AND paid_at IS NULL
			AND (approved IS NULL OR approved = true)
		RETURNING id`

	overdueRows, err := is.postgres.Query(ctx, overdueQuery)
	if err != nil {
		is.log.Error("Failed to update overdue invoices", "error", err)
		return err
	}
	defer overdueRows.Close()

	var overdueIDs []string
	for overdueRows.Next() {
		var invoiceID string
		if err := overdueRows.Scan(&invoiceID); err != nil {
			is.log.Error("Failed to scan overdue invoice ID", "error", err)
			continue
		}
		overdueIDs = append(overdueIDs, invoiceID)
		is.DeleteFromRedisCache(ctx, invoiceID)
	}

	if err := overdueRows.Err(); err != nil {
		is.log.Error("Row iteration error", "error", err)
		return err
	}

	is.log.Info("Updated invoice statuses",
		"cancelled_count", len(cancelledIDs),
		"overdue_count", len(overdueIDs),
	)

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

func (is *InvoiceService) GetStats(ctx context.Context, userId string) *utils.ApiResponse {
	const query = `
		SELECT 
			COUNT(*) FILTER (WHERE status = 'pending') AS pending_count,
			COUNT(*) FILTER (WHERE status = 'paid') AS paid_count,
			COUNT(*) FILTER (WHERE status = 'overdue') AS overdue_count,
			COUNT(*) AS total_count
		FROM invoice
		WHERE created_by_id = $1;
	`

	var stats struct {
		PendingCount int `db:"pending_count" json:"pending_count"`
		PaidCount    int `db:"paid_count" json:"paid_count"`
		OverdueCount int `db:"overdue_count" json:"overdue_count"`
		TotalCount   int `db:"total_count" json:"total_count"`
	}

	err := is.postgres.QueryRow(ctx, query, userId).Scan(&stats.PendingCount, &stats.PaidCount, &stats.OverdueCount, &stats.TotalCount)
	if err != nil {
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to query status",
		}
	}

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "successful",
		Data:       stats,
	}
}

func (ir *InvoiceService) GetInvoiceCountByStatusQuery(ctx context.Context, userID string, startDate, endDate time.Time) ([]InvoiceStatusRow, error) {
	const query = `
        SELECT 
            status::text as status,
            COUNT(*) as count
        FROM invoice
        WHERE created_by_id = $1
            AND created_at >= $2
            AND created_at < $3
        GROUP BY status
        ORDER BY status
    `

	rows, err := ir.postgres.Query(ctx, query, userID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to query invoice status counts: %w", err)
	}
	defer rows.Close()

	var results []InvoiceStatusRow
	for rows.Next() {
		var row InvoiceStatusRow
		if err := rows.Scan(&row.Status, &row.Count); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		results = append(results, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return results, nil
}

func (is *InvoiceService) GetInvoicesByStatus(ctx context.Context, userID string, query InvoiceStatusQuery) *utils.ApiResponse {
	var targetMonth time.Month
	var targetYear int
	var err error

	now := time.Now()

	if query.Month == "" {
		targetMonth = now.Month()
		targetYear = now.Year()
	} else {
		targetMonth, err = utils.ParseMonthString(query.Month)
		if err != nil {
			is.log.Error("failed to parse month for invoice status query", "error", err, "query_value", query)
			return &utils.ApiResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "invalid month format. Use full name (July), abbreviation (Jul), or number (07)",
			}
		}

		targetYear = now.Year()
		if targetMonth > now.Month() {
			targetYear--
		}
	}

	startDate := time.Date(targetYear, targetMonth, 1, 0, 0, 0, 0, time.UTC)
	endDate := startDate.AddDate(0, 1, 0)

	statusData, err := is.GetInvoiceCountByStatusQuery(ctx, userID, startDate, endDate)
	if err != nil {
		is.log.Error("failed to return data from postgres", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "failed to retrieve invoice status data",
		}
	}

	invoicesByStatus := make([]InvoiceStatusDataPoint, 0, len(statusData))

	for _, row := range statusData {
		invoicesByStatus = append(invoicesByStatus, InvoiceStatusDataPoint{
			Status: utils.CapitalizeStatus(row.Status),
			Value:  row.Count,
		})
	}

	if len(invoicesByStatus) == 0 {
		invoicesByStatus = []InvoiceStatusDataPoint{}
	}

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "successful",
		Data:       invoicesByStatus,
	}
}

func (is *InvoiceService) GetNameFromEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) > 0 && parts[0] != "" {
		username := parts[0]
		sanitized := strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
				(r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' {
				return r
			}
			return -1
		}, username)

		if sanitized != "" {
			return sanitized
		}
	}
	return fmt.Sprintf("user_%s", uuid.New().String()[:8])
}

func (is *InvoiceService) UpdateInvoice(ctx context.Context, invoiceID, userID string, dto UpdateInvoiceDTO, logoFile *logo.LogoFileData) *utils.ApiResponse {
	if _, err := uuid.Parse(invoiceID); err != nil {
		return &utils.ApiResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "Invalid invoice ID format",
		}
	}

	const checkQuery = `
		SELECT id, status, created_by_id, logo_id
  		FROM invoice
		WHERE id = $1
    	AND status NOT IN ('sent', 'viewed', 'paid', 'overdue')
	`

	var existingInvoiceID, status, ownerID string
	var existingLogoID sql.NullString

	err := is.postgres.QueryRow(ctx, checkQuery, invoiceID).Scan(&existingInvoiceID, &status, &ownerID, &existingLogoID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return &utils.ApiResponse{
				StatusCode: http.StatusNotFound,
				Message:    "Invoice not found",
			}
		}
		is.log.Error("failed to fetch invoice", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to process request",
		}
	}

	if ownerID != userID {
		return &utils.ApiResponse{
			StatusCode: http.StatusForbidden,
			Message:    "You don't have permission to update this invoice",
		}
	}

	tx, err := is.postgres.Begin(ctx)
	if err != nil {
		is.log.Error("failed to begin transaction", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to process request",
		}
	}
	defer tx.Rollback(ctx)

	var logoID sql.NullString

	if logoFile != nil {
		createLogo, err := is.logoService.UploadAndCreateLogo(ctx, tx, userID, logoFile)
		if err != nil {
			is.log.Error("failed to upload logo", "error", err)
			return &utils.ApiResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    "Failed to upload logo",
			}
		}
		// logo has been created, just update the logo id in the invoice database
		logoID = sql.NullString{String: createLogo.LogoID, Valid: true}
	} else {
		logoID = existingLogoID
		// if existingLogoID.Valid {
		// url, _ := is.logoService.GetSignedDownloadURL(ctx, existingLogoID.String)
		// logoURL = sql.NullString{String: url, Valid: true}
		// }
	}

	updates := []string{}
	args := []interface{}{invoiceID}
	argPosition := 2

	if dto.Title != "" {
		updates = append(updates, fmt.Sprintf("title = $%d", argPosition))
		args = append(args, dto.Title)
		argPosition++
	}

	if dto.RecipientName != "" {
		updates = append(updates, fmt.Sprintf("payer_name = $%d", argPosition))
		args = append(args, dto.RecipientName)
		argPosition++
	}

	if dto.Email != "" {
		updates = append(updates, fmt.Sprintf("payer_email = $%d", argPosition))
		args = append(args, dto.Email)
		argPosition++
	}

	if dto.Country != "" {
		updates = append(updates, fmt.Sprintf("address_country = $%d", argPosition))
		args = append(args, dto.Country)
		argPosition++
	}

	if dto.DueDate != "" {
		dueDate, err := time.Parse("2006-01-02", dto.DueDate)
		if err != nil {
			return &utils.ApiResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "Invalid due date format. Use YYYY-MM-DD",
			}
		}

		if dueDate.Before(time.Now().Truncate(24 * time.Hour)) {
			return &utils.ApiResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "Due date cannot be in the past",
			}
		}

		updates = append(updates, fmt.Sprintf("due_date = $%d", argPosition))
		args = append(args, dueDate)
		argPosition++
	}

	if dto.TemplateID != "" {
		updates = append(updates, fmt.Sprintf("template_id = $%d", argPosition))
		args = append(args, dto.TemplateID)
		argPosition++
	}

	if len(dto.InvoiceItems) > 0 || dto.ServiceFee != nil {
		var subtotal float64
		var serviceFee float64

		if len(dto.InvoiceItems) > 0 {
			for _, item := range dto.InvoiceItems {
				subtotal += item.Amount
			}
		} else {
			const subQuery = `SELECT sub_total FROM invoice WHERE id = $1`
			tx.QueryRow(ctx, subQuery, invoiceID).Scan(&subtotal)
		}

		if dto.ServiceFee != nil {
			serviceFee = *dto.ServiceFee
		} else {
			const feeQuery = `SELECT service_fee FROM invoice WHERE id = $1`
			tx.QueryRow(ctx, feeQuery, invoiceID).Scan(&serviceFee)
		}

		total := subtotal + serviceFee

		updates = append(updates, fmt.Sprintf("sub_total = $%d", argPosition))
		args = append(args, subtotal)
		argPosition++

		updates = append(updates, fmt.Sprintf("service_fee = $%d", argPosition))
		args = append(args, serviceFee)
		argPosition++

		updates = append(updates, fmt.Sprintf("total = $%d", argPosition))
		args = append(args, total)
		argPosition++
	}

	if logoFile != nil {
		updates = append(updates, fmt.Sprintf("logo_id = $%d", argPosition))
		args = append(args, logoID)
		argPosition++
	}

	updates = append(updates, "updated_at = NOW()")

	if len(updates) == 1 {
		return &utils.ApiResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "No fields to update",
		}
	}

	updateQuery := fmt.Sprintf(`
		UPDATE invoice 
		SET %s
		WHERE id = $1
		RETURNING updated_at
	`, strings.Join(updates, ", "))

	var updatedAt time.Time
	err = tx.QueryRow(ctx, updateQuery, args...).Scan(&updatedAt)
	if err != nil {
		is.log.Error("failed to update invoice", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to update invoice",
		}
	}

	if len(dto.InvoiceItems) > 0 {
		const deleteItemsQuery = `DELETE FROM invoice_items WHERE invoice_id = $1`
		_, err = tx.Exec(ctx, deleteItemsQuery, invoiceID)
		if err != nil {
			is.log.Error("failed to delete old invoice items", "error", err)
			return &utils.ApiResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    "Failed to update invoice items",
			}
		}

		const itemQuery = `
			INSERT INTO invoice_items (
				invoice_id, item_type, description, quantity, unit_price, discount, amount
			) VALUES($1, $2, $3, $4, $5, $6, $7)`

		for _, item := range dto.InvoiceItems {
			_, err = tx.Exec(ctx, itemQuery,
				invoiceID,
				item.InvoiceType,
				item.Description,
				item.Quantity,
				item.UnitPrice,
				item.Discount,
				item.Amount,
			)
			if err != nil {
				is.log.Error("failed to insert invoice item", "error", err)
				return &utils.ApiResponse{
					StatusCode: http.StatusInternalServerError,
					Message:    "Failed to update invoice items",
				}
			}
		}
	}

	if err = tx.Commit(ctx); err != nil {
		is.log.Error("failed to commit transaction", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to complete update",
		}
	}

	cacheKey := fmt.Sprintf("invoice:%s", invoiceID)
	_ = is.DeleteFromRedisCache(ctx, cacheKey)

	is.log.Info("invoice updated successfully", "invoice_id", invoiceID)

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "Invoice updated successfully",
		Data: map[string]interface{}{
			"invoice_id": invoiceID,
			"updated_at": updatedAt,
		},
	}
}

func (is *InvoiceService) handleInvoiceNotFound(ctx context.Context, invoiceId, userId string) *utils.ApiResponse {
	var exists bool
	var isOwner bool

	err := is.postgres.QueryRow(ctx, `
		SELECT 
			EXISTS(SELECT 1 FROM invoice WHERE id = $1),
			EXISTS(SELECT 1 FROM invoice WHERE id = $1 AND created_by_id = $2)
	`, invoiceId, userId).Scan(&exists, &isOwner)

	if err != nil || !exists {
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

func (is *InvoiceService) GetInvoiceItems(ctx context.Context, invoiceID string) ([]InvoiceItems, error) {
	rows, err := is.postgres.Query(ctx, `
		SELECT id, item_type, description, quantity, unit_price, COALESCE(discount, 0), amount, created_at
		FROM invoice_items WHERE invoice_id = $1 ORDER BY created_at
	`, invoiceID)
	if err != nil {
		return nil, fmt.Errorf("failed to query invoice items: %w", err)
	}
	defer rows.Close()

	var items []InvoiceItems
	for rows.Next() {
		var item InvoiceItems
		var createdAt time.Time
		if err := rows.Scan(&item.ItemId, &item.InvoiceType, &item.Description, &item.Quantity, &item.UnitPrice, &item.Discount, &item.Amount, &createdAt); err != nil {
			return nil, fmt.Errorf("failed to scan invoice item: %w", err)
		}
		item.CreatedAt = &createdAt
		items = append(items, item)
	}
	return items, nil
}

func (is *InvoiceService) GenerateInvoiceHTML(ctx context.Context, invoiceID, userID string) ([]byte, string, error) {
	type invoiceRow struct {
		invoiceNumber   string
		status          string
		payerEmail      string
		payerName       string
		currency        string
		subTotal        float64
		serviceFee      float64
		total           float64
		createdAt       time.Time
		dueDate         sql.NullTime
		note            sql.NullString
		creatorEmail    string
		creatorFirst    string
		creatorLast     string
		creatorBiz      sql.NullString
		creatorPhone    sql.NullString
		creatorCountry  sql.NullString
		country         sql.NullString
	}

	var r invoiceRow
	err := is.postgres.QueryRow(ctx, `
		SELECT i.invoice_number, i.status, i.payer_email, i.payer_name, i.currency,
		       i.sub_total, i.service_fee, i.total, i.created_at, i.due_date, i.notes,
		       u.email, u.first_name, u.last_name, u.business_name, u.phone_number, u.country,
		       i.address_country
		FROM invoice i
		JOIN users u ON u.id = i.created_by_id
		WHERE i.id = $1 AND i.created_by_id = $2
	`, invoiceID, userID).Scan(
		&r.invoiceNumber, &r.status, &r.payerEmail, &r.payerName, &r.currency,
		&r.subTotal, &r.serviceFee, &r.total, &r.createdAt, &r.dueDate, &r.note,
		&r.creatorEmail, &r.creatorFirst, &r.creatorLast, &r.creatorBiz, &r.creatorPhone, &r.creatorCountry,
		&r.country,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, "", fmt.Errorf("invoice not found")
		}
		return nil, "", fmt.Errorf("failed to fetch invoice: %w", err)
	}

	items, err := is.GetInvoiceItems(ctx, invoiceID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch invoice items: %w", err)
	}

	creatorName := strings.TrimSpace(fmt.Sprintf("%s %s", r.creatorFirst, r.creatorLast))
	dueDateStr := ""
	if r.dueDate.Valid {
		dueDateStr = r.dueDate.Time.Format("02 Jan 2006")
	}

	statusClass := strings.ToLower(r.status)

	data := map[string]any{
		"InvoiceNumber":   r.invoiceNumber,
		"Status":          strings.ToUpper(r.status[:1]) + r.status[1:],
		"StatusClass":     statusClass,
		"CreatorName":     creatorName,
		"CreatorEmail":    r.creatorEmail,
		"CreatorBusiness": r.creatorBiz.String,
		"CreatorPhone":    r.creatorPhone.String,
		"CreatorCountry":  r.creatorCountry.String,
		"PayerName":       r.payerName,
		"PayerEmail":      r.payerEmail,
		"PayerCountry":    r.country.String,
		"Currency":        strings.ToUpper(r.currency),
		"SubTotal":        r.subTotal,
		"ServiceFee":      r.serviceFee,
		"Total":           r.total,
		"CreatedAt":       r.createdAt.Format("02 Jan 2006"),
		"DueDate":         dueDateStr,
		"Note":            r.note.String,
		"GeneratedAt":     time.Now().UTC().Format("02 Jan 2006, 15:04 UTC"),
		"Items":           items,
	}

	html, err := mail.RenderInvoicePDF(data)
	if err != nil {
		return nil, "", fmt.Errorf("failed to render invoice HTML: %w", err)
	}
	return html, r.invoiceNumber, nil
}

func (is *InvoiceService) MarkInvoicePaid(ctx context.Context, invoiceID, userID string, dto MarkInvoicePaidDTO) *utils.ApiResponse {
	var ownerID, status, invoiceNumber, currency string
	var total float64
	err := is.postgres.QueryRow(ctx, `
		SELECT created_by_id, status, invoice_number, total, currency
		FROM invoice WHERE id = $1
	`, invoiceID).Scan(&ownerID, &status, &invoiceNumber, &total, &currency)
	if err != nil {
		if err == pgx.ErrNoRows {
			return &utils.ApiResponse{StatusCode: http.StatusNotFound, Message: "invoice not found"}
		}
		return &utils.ApiResponse{StatusCode: http.StatusInternalServerError, Message: "failed to fetch invoice"}
	}

	if ownerID != userID {
		return &utils.ApiResponse{StatusCode: http.StatusForbidden, Message: "access denied"}
	}

	if status == string(InvoiceStatusPaid) {
		return &utils.ApiResponse{StatusCode: http.StatusBadRequest, Message: "invoice is already marked as paid"}
	}
	if status == string(InvoiceStatusCancelled) {
		return &utils.ApiResponse{StatusCode: http.StatusBadRequest, Message: "cannot mark a cancelled invoice as paid"}
	}

	tx, err := is.postgres.Begin(ctx)
	if err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusInternalServerError, Message: "failed to begin transaction"}
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		UPDATE invoice SET status = 'paid', paid_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, invoiceID)
	if err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusInternalServerError, Message: "failed to update invoice status"}
	}

	walletID := sql.NullString{}
	if dto.WalletID != "" {
		walletID = sql.NullString{String: dto.WalletID, Valid: true}
	}

	var txID string
	err = tx.QueryRow(ctx, `
		INSERT INTO transactions (invoice_id, wallet_id, transaction_hash, amount, currency, status, network_fee, token_type, transaction_type, user_id, confirmed_at)
		VALUES ($1, $2, $3, $4, $5, 'confirmed', $6, $5, 'payment', $7, NOW())
		RETURNING id
	`, invoiceID, walletID, dto.TransactionHash, dto.Amount, currency, dto.NetworkFee, userID).Scan(&txID)
	if err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusInternalServerError, Message: "failed to record transaction"}
	}

	if err := tx.Commit(ctx); err != nil {
		return &utils.ApiResponse{StatusCode: http.StatusInternalServerError, Message: "failed to commit"}
	}

	is.DeleteFromRedisCache(ctx, invoiceID)

	go func() {
		bgCtx := context.Background()
		notifBody := fmt.Sprintf("Invoice #%s has been marked as paid. Amount: %.2f %s", invoiceNumber, dto.Amount, strings.ToUpper(currency))
		notifications.NewNotificationService().CreateNewNotification(bgCtx, notifications.CreateNotificationDto{
			Title:  "Invoice Paid",
			UserId: userID,
			Body:   notifBody,
		})
	}()

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "invoice marked as paid",
		Data: map[string]any{
			"invoice_id":     invoiceID,
			"transaction_id": txID,
			"status":         "paid",
		},
	}
}
