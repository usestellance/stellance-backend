package invoice

import (
	"encoding/json"
	"net/http"
	"time"

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
