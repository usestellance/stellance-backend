package invoice

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/The-True-Hooha/stellance-backend.git/internal/user"
	"github.com/The-True-Hooha/stellance-backend.git/pkg/utils"
	"github.com/go-playground/validator/v10"
)

type InvoiceHandler struct {
	service   *InvoiceService
	validator validator.Validate
}

func NewInvoiceHandler(is *InvoiceService) *InvoiceHandler {
	return &InvoiceHandler{
		service:   is,
		validator: *validator.New(),
	}
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
	}

	pageCount := r.URL.Query().Get("page_size")
	if pageCount != "" {
		if ps, err := strconv.Atoi(pageCount); err == nil {
			dto.Count = ps
		}
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

func (h *InvoiceHandler) GetInvoiceByID(w http.ResponseWriter, r *http.Request) {
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

	invoiceID := r.URL.Query().Get("id")
	invoiceUrl := r.URL.Query().Get("url")

	if (invoiceID == "" && invoiceUrl == "") || (invoiceID != "" && invoiceUrl != "") {
		http.Error(w, "You can only provide either 'id' or 'url', not both or none", http.StatusBadRequest)
		return
	}

	if invoiceID != "" {
		response := h.service.GetInvoiceById(ctx, invoiceID, reqUserId, role)
		utils.WriteToJson(w, response.StatusCode, response)
		return
	}

	response := h.service.GetInvoiceByUrl(ctx, invoiceUrl, reqUserId, role)
	utils.WriteToJson(w, response.StatusCode, response)

}
