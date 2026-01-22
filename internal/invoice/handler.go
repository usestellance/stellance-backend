package invoice

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/The-True-Hooha/stellance-backend/internal/logo"
	"github.com/The-True-Hooha/stellance-backend/internal/user"
	"github.com/The-True-Hooha/stellance-backend/pkg/utils"
	"github.com/go-playground/validator/v10"
)

type InvoiceHandler struct {
	service   *InvoiceService
	validator *validator.Validate
}

func NewInvoiceHandler(is *InvoiceService) *InvoiceHandler {
	v := validator.New()
	RegisterInvoiceFiltersValidation(v)
	return &InvoiceHandler{
		service:   is,
		validator: v,
	}
}

func RegisterInvoiceFiltersValidation(v *validator.Validate) {
	v.RegisterValidation("invoice_status", func(fl validator.FieldLevel) bool {
		status := fl.Field().String()
		switch status {
		case "", "draft", "sent", "viewed", "paid", "overdue", "cancelled", "refunded":
			return true
		default:
			return false
		}
	})

	v.RegisterValidation("order_by", func(fl validator.FieldLevel) bool {
		order := strings.ToLower(fl.Field().String())
		return order == "asc" || order == "desc"
	})
}

func (handler *InvoiceHandler) CreateNewInvoiceHandler(w http.ResponseWriter, r *http.Request) {
	log := handler.service.log
	ctx := r.Context()

	userID, ok := utils.GetUserIDFromContext(ctx)
	if !ok {
		http.Error(w, "invalid request! not allowed", http.StatusUnauthorized)
		return
	}

	err := r.ParseMultipartForm(10 << 20)
	if err != nil {
		log.Error("failed to parse form data", "error", err)
		http.Error(w, "incomplete form data: failed to parse form data", http.StatusBadRequest)
		return
	}

	var logoFileData *logo.LogoFileData
	file, fileHeader, err := r.FormFile("logo")
	if err == nil {
		defer file.Close()
		contentType := fileHeader.Header.Get("Content-Type")
		validTypes := map[string]bool{
			"image/png":  true,
			"image/jpeg": true,
			"image/jpg":  true,
		}

		if !validTypes[contentType] {
			utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "invalid file type. Only PNG, JPEG, JPG are allowed",
			})
			return
		}

		if fileHeader.Size > 2*1024*1024 {
			utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "file size exceeds maximum allowed size of 2MB",
			})
			return
		}

		makeDefault := r.FormValue("make_default") == "true"

		logoFileData = &logo.LogoFileData{
			File:        file,
			FileHeader:  fileHeader,
			MakeDefault: makeDefault,
		}

	} else if err != http.ErrMissingFile {
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "error processing logo file",
			Error:      err.Error(),
		})
		return
	}

	var dto CreateInvoiceDTO

	dto.Title = r.FormValue("title")
	dto.RecipientName = r.FormValue("payer_name")
	dto.Email = r.FormValue("payer_email")
	dto.Country = r.FormValue("country")
	dto.DueDate = r.FormValue("due_date")
	dto.TemplateID = TemplateIDType(r.FormValue("template_id"))
	dto.Note = r.FormValue("note")

	serviceFeeStr := r.FormValue("service_fee")
	if serviceFeeStr != "" {
		serviceFee, err := strconv.ParseFloat(serviceFeeStr, 64)
		if err != nil {
			utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "invalid service_fee value",
			})
			return
		}
		dto.ServiceFee = serviceFee
	}

	invoiceItemsJSON := r.FormValue("invoice_items")
	if invoiceItemsJSON == "" {
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "invoice_items is required",
		})
		return
	}

	err = json.Unmarshal([]byte(invoiceItemsJSON), &dto.InvoiceItems)
	if err != nil {
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "invalid invoice_items format",
			Error:      err.Error(),
		})
		return
	}

	if len(dto.InvoiceItems) == 0 {
		http.Error(w, "At least one invoice item is required", http.StatusBadRequest)
		return
	}

	if err := handler.validator.Struct(dto); err != nil {
		utils.HandleValidationError(w, err)
		return
	}

	dueDate, err := time.Parse("2006-01-02", dto.DueDate)
	if err != nil {
		http.Error(w, "Invalid due date format. Use YYYY-MM-DD", http.StatusBadRequest)
		return
	}

	if dueDate.Before(time.Now().Truncate(24 * time.Hour)) {
		http.Error(w, "Due date cannot be in the past", http.StatusBadRequest)
		return
	}

	response := handler.service.GenerateNewInvoice(ctx, dto, userID, logoFileData)
	utils.WriteToJson(w, response.StatusCode, response)
}

func (handler *InvoiceHandler) GetManyInvoiceHandler(w http.ResponseWriter, r *http.Request) {

	log := handler.service.log
	ctx := r.Context()

	reqUserId, ok := utils.GetUserIDFromContext(ctx)
	if !ok {
		http.Error(w, "invalid request! not allowed", http.StatusUnauthorized)
		return
	}

	role, ok := utils.GetRoleFromContext(ctx)
	if !ok {
		http.Error(w, "Unauthorized: missing role", http.StatusUnauthorized)
		return
	}

	userID := r.URL.Query().Get("user_id")

	if userID == "" {
		userID = reqUserId
	}

	if userID != reqUserId && role != string(user.RoleAdmin) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	var queryUserId string
	if role == string(user.RoleAdmin) {
		queryUserId = userID
	} else {
		queryUserId = reqUserId
	}

	dto := InvoiceFiltersDto{
		Status: InvoiceStatus(r.URL.Query().Get("status")),
		UserId: queryUserId,
	}

	page := r.URL.Query().Get("page")
	if page != "" {
		if p, err := strconv.Atoi(page); err == nil {
			dto.Page = p
		}
	} else {
		dto.Page = 1
	}

	pageCount := r.URL.Query().Get("page_count")
	if pageCount != "" {
		if ps, err := strconv.Atoi(pageCount); err == nil {
			dto.Count = ps
		}
	} else {
		dto.Count = 10
	}

	orderBy := r.URL.Query().Get("order_by")
	if orderBy == "" {
		dto.OrderBy = utils.OrderByDESC
	} else {
		dto.OrderBy = utils.OrderByType(orderBy)
	}

	log.Info("fetching invoices",
		"user_id", userID,
		"filters", dto,
	)

	response := handler.service.GetManyInvoice(ctx, dto, queryUserId)
	utils.WriteToJson(w, response.StatusCode, response)

}

func (h *InvoiceHandler) GetInvoiceByIDHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	reqUserId, ok := utils.GetUserIDFromContext(ctx)
	if !ok {
		http.Error(w, "invalid request! not allowed", http.StatusUnauthorized)
		return
	}

	role, ok := utils.GetRoleFromContext(ctx)
	if !ok {
		http.Error(w, "Unauthorized: missing role", http.StatusUnauthorized)
		return
	}
	invoiceID := r.PathValue("id")
	response := h.service.GetInvoiceById(ctx, invoiceID, reqUserId, role)
	utils.WriteToJson(w, response.StatusCode, response)
}

func (h *InvoiceHandler) GetInvoiceSearchHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	reqUserId, ok := utils.GetUserIDFromContext(ctx)
	if !ok {
		h.service.log.Warn("public access to invoice")
	}

	role, ok := utils.GetRoleFromContext(ctx)
	if !ok {
		h.service.log.Warn("public access to invoice")
	}

	invoiceID := r.URL.Query().Get("id")
	invoiceUrl := r.URL.Query().Get("url")

	if (invoiceID == "" && invoiceUrl == "") || (invoiceID != "" && invoiceUrl != "") {
		http.Error(w, "You can only provide either 'id' or 'url', not both or none", http.StatusBadRequest)
		return
	}
	response := h.service.GetInvoiceSearch(ctx, invoiceUrl, invoiceID, reqUserId, role)
	utils.WriteToJson(w, response.StatusCode, response)

}

func (h *InvoiceHandler) QueryInvoiceBySearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := utils.GetUserIDFromContext(ctx)
	if !ok {
		h.service.log.Warn("public access to invoice")
	}

	// role, ok := utils.GetRoleFromContext(ctx)
	// if !ok {
	// 	h.service.log.Warn("public access to invoice")
	// }

	search := r.URL.Query().Get("search")

	response := h.service.GetInvoiceBySearchOnUser(ctx, search, userID, 0, 0)
	utils.WriteToJson(w, response.StatusCode, response)

}

func (h *InvoiceHandler) DeleteInvoiceHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	reqUserId, ok := utils.GetUserIDFromContext(ctx)
	if !ok {
		http.Error(w, "invalid request! not allowed", http.StatusUnauthorized)
		return
	}
	invoiceID := r.PathValue("id")

	response := h.service.DeleteInvoice(ctx, reqUserId, invoiceID)
	utils.WriteToJson(w, response.StatusCode, response)

}

func (h *InvoiceHandler) EditInvoiceHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	reqUserId, ok := utils.GetUserIDFromContext(ctx)
	if !ok {
		http.Error(w, "invalid request! not allowed", http.StatusUnauthorized)
		return
	}
	invoiceID := r.PathValue("id")
	var dto CreateInvoiceDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		h.service.log.Error("failed to decode request", "error", err)
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if err := h.validator.Struct(dto); err != nil {
		utils.HandleValidationError(w, err)
		return
	}

	if len(dto.InvoiceItems) == 0 {
		http.Error(w, "At least one invoice item is required", http.StatusBadRequest)
		return
	}

	dueDate, err := time.Parse("2006-01-02", dto.DueDate)
	if err != nil {
		http.Error(w, "Invalid due date format. Use YYYY-MM-DD", http.StatusBadRequest)
		return
	}

	if dueDate.Before(time.Now().Truncate(24 * time.Hour)) {
		http.Error(w, "Due date cannot be in the past", http.StatusBadRequest)
		return
	}
	response := h.service.EditInvoice(ctx, reqUserId, invoiceID, dto)
	utils.WriteToJson(w, response.StatusCode, response)

}

func (h *InvoiceHandler) SendInvoice(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	reqUserId, ok := utils.GetUserIDFromContext(ctx)
	if !ok {
		http.Error(w, "invalid request! not allowed", http.StatusUnauthorized)
		return
	}
	invoiceID := r.PathValue("id")

	var dto SendInvoiceDto
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "invalid request body",
			Error:      err.Error(),
		})
		return
	}

	if err := h.validator.Struct(dto); err != nil {
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "request failed",
			Error:      err.Error(),
		})
		return
	}

	response := h.service.SendInvoice(ctx, reqUserId, invoiceID, dto.Emails)
	utils.WriteToJson(w, response.StatusCode, response)
}

func (h *InvoiceHandler) ReviewInvoiceHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	approveStr := r.URL.Query().Get("approve")
	invoiceID := r.PathValue("id")

	reqUserId, ok := utils.GetUserIDFromContext(ctx)
	if !ok {
		h.service.log.Warn("public access to invoice")
	}

	role, ok := utils.GetRoleFromContext(ctx)
	if !ok {
		h.service.log.Warn("public access to invoice")
	}

	approve, err := strconv.ParseBool(approveStr)
	if err != nil {
		http.Error(w, "Invalid approve value", http.StatusBadRequest)
		return
	}

	response := h.service.ReviewInvoice(ctx, invoiceID, approve, role, reqUserId)
	utils.WriteToJson(w, response.StatusCode, response)
}

func (h *InvoiceHandler) GetInvoiceStatsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userId, ok := utils.GetUserIDFromContext(ctx)
	if !ok {
		h.service.log.Warn("public access to invoice")
	}
	response := h.service.GetStats(ctx, userId)
	utils.WriteToJson(w, response.StatusCode, response)
}

func (ih *InvoiceHandler) GetInvoicesByStatus(w http.ResponseWriter, r *http.Request) {

	userID, ok := utils.GetUserIDFromContext(r.Context())
	if !ok {
		utils.WriteToJson(w, http.StatusUnauthorized, utils.ApiResponse{
			StatusCode: http.StatusUnauthorized,
			Message:    "unauthorized",
		})
		return
	}

	var query InvoiceStatusQuery
	query.Month = r.URL.Query().Get("month")

	response := ih.service.GetInvoicesByStatus(r.Context(), userID, query)

	utils.WriteToJson(w, response.StatusCode, response)
}

func (handler *InvoiceHandler) UpdateInvoiceHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	invoiceID := r.PathValue("id")

	userID, ok := utils.GetUserIDFromContext(ctx)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	err := r.ParseMultipartForm(10 << 20)
	if err != nil {
		handler.service.log.Error("failed to parse form data", "error", err)
		http.Error(w, "failed to parse form data", http.StatusBadRequest)
		return
	}

	var logoFileData *logo.LogoFileData
	file, fileHeader, err := r.FormFile("logo")
	if err == nil {
		defer file.Close()
		contentType := fileHeader.Header.Get("Content-Type")
		validTypes := map[string]bool{
			"image/png":  true,
			"image/jpeg": true,
			"image/jpg":  true,
		}

		if !validTypes[contentType] {
			utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "invalid file type. Only PNG, JPEG, JPG are allowed",
			})
			return
		}

		if fileHeader.Size > 2*1024*1024 {
			utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "file size exceeds maximum allowed size of 2MB",
			})
			return
		}

		makeDefault := r.FormValue("make_default") == "true"
		logoFileData = &logo.LogoFileData{
			File:        file,
			FileHeader:  fileHeader,
			MakeDefault: makeDefault,
		}
	} else if err != http.ErrMissingFile {
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "error processing logo file",
			Error:      err.Error(),
		})
		return
	}

	var dto UpdateInvoiceDTO

	if title := r.FormValue("title"); title != "" {
		dto.Title = title
	}
	if payerName := r.FormValue("payer_name"); payerName != "" {
		dto.RecipientName = payerName
	}
	if email := r.FormValue("payer_email"); email != "" {
		dto.Email = email
	}
	if country := r.FormValue("country"); country != "" {
		dto.Country = country
	}
	if dueDate := r.FormValue("due_date"); dueDate != "" {
		dto.DueDate = dueDate
	}
	if templateID := r.FormValue("template_id"); templateID != "" {
		dto.TemplateID = TemplateIDType(templateID)
	}

	if serviceFeeStr := r.FormValue("service_fee"); serviceFeeStr != "" {
		serviceFee, err := strconv.ParseFloat(serviceFeeStr, 64)
		if err != nil {
			utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "invalid service_fee value",
			})
			return
		}
		dto.ServiceFee = &serviceFee
	}

	if invoiceItemsJSON := r.FormValue("invoice_items"); invoiceItemsJSON != "" {
		err = json.Unmarshal([]byte(invoiceItemsJSON), &dto.InvoiceItems)
		if err != nil {
			utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "invalid invoice_items format",
				Error:      err.Error(),
			})
			return
		}
	}

	if err := handler.validator.Struct(dto); err != nil {
		utils.HandleValidationError(w, err)
		return
	}

	response := handler.service.UpdateInvoice(ctx, invoiceID, userID, dto, logoFileData)
	utils.WriteToJson(w, response.StatusCode, response)
}
