package transactions

import (
	"net/http"
	"strconv"

	"github.com/The-True-Hooha/stellance-backend/internal/user"
	"github.com/The-True-Hooha/stellance-backend/pkg/utils"
	"github.com/go-playground/validator/v10"
)

type TransactionHandler struct {
	service   *TransactionService
	validator *validator.Validate
}

func NewTransactionHandler(ts *TransactionService) *TransactionHandler {
	v := validator.New()
	return &TransactionHandler{
		service:   ts,
		validator: v,
	}
}

func(h *TransactionHandler)GetTransactionOverviewCard(w http.ResponseWriter, r *http.Request){
	userId, ok := utils.GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized request", http.StatusUnauthorized)
		return 
	}
	res := h.service.GetTransactionCardForUser(r.Context(), userId)
	utils.WriteToJson(w, res.StatusCode, res)
}

func (h *TransactionHandler) GetTransactionByIdHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	requestingUserID, ok := utils.GetUserIDFromContext(ctx)
	if !ok {
		http.Error(w, "Unauthorized: missing user ID", http.StatusUnauthorized)
		return
	}
	response := h.service.GetTransactionByID(ctx, id, requestingUserID)
	utils.WriteToJson(w, response.StatusCode, response)
}

func (h *TransactionHandler) DeleteTransactionByIdHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	requestingUserID, ok := utils.GetUserIDFromContext(ctx)
	if !ok {
		http.Error(w, "Unauthorized: missing user ID", http.StatusUnauthorized)
		return
	}
	response := h.service.DeleteTransactionByID(ctx, id, requestingUserID)
	utils.WriteToJson(w, response.StatusCode, response)
}

func (handler *TransactionHandler) GetManyTransactionHandler(w http.ResponseWriter, r *http.Request) {

	log := handler.service.log
	ctx := r.Context()

	reqUserId, ok := utils.GetUserIDFromContext(ctx)
	if !ok {
		http.Error(w, "invalid request! not allowed", http.StatusUnauthorized)
		return
	}
	userID := r.URL.Query().Get("user_id")

	if userID == "" {
		userID = reqUserId
	}

	role, ok := utils.GetRoleFromContext(ctx)
	if !ok {
		http.Error(w, "Unauthorized: missing role", http.StatusUnauthorized)
		return
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

	dto := TransactionFiltersDto{
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

	log.Info("fetching invoices",
		"user_id", userID,
		"filters", dto,
	)

	response := handler.service.GetTransactionsPaginated(ctx, dto.Page, dto.Count, dto.UserId)
	utils.WriteToJson(w, response.StatusCode, response)

}
