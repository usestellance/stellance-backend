package invoice

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/The-True-Hooha/stellance-backend.git/internal/user"
	"github.com/The-True-Hooha/stellance-backend.git/pkg/utils"
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

	var dto CreateInvoiceDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		log.Error("failed to decode request", "error", err)
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if err := handler.validator.Struct(dto); err != nil {
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

	response := handler.service.GenerateNewInvoice(ctx, dto, userID)
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
	var email string
	if r.URL.Query().Get("email") != "" {
		email = r.URL.Query().Get("email")
	}
	response := h.service.SendInvoice(ctx, reqUserId, invoiceID, email)
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
