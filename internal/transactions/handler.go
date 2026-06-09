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

// GetTransactionOverviewCard godoc
// @Summary      Get transaction overview stats
// @Tags         transactions
// @Produce      json
// @Success      200  {object}  utils.ApiResponse
// @Failure      401  {object}  utils.ApiResponse
// @Security     BearerAuth
// @Router       /transactions/stats [get]
func (h *TransactionHandler) GetTransactionOverviewCard(w http.ResponseWriter, r *http.Request) {
	userId, ok := utils.GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized request", http.StatusUnauthorized)
		return
	}
	res := h.service.GetTransactionCardForUser(r.Context(), userId)
	utils.WriteToJson(w, res.StatusCode, res)
}

// GetTransactionByIdHandler godoc
// @Summary      Get transaction by ID
// @Tags         transactions
// @Produce      json
// @Param        id  path  string  true  "Transaction ID"
// @Success      200  {object}  utils.ApiResponse
// @Failure      401  {object}  utils.ApiResponse
// @Security     BearerAuth
// @Router       /transactions/id/{id} [get]
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

// DeleteTransactionByIdHandler godoc
// @Summary      Delete a transaction
// @Tags         transactions
// @Produce      json
// @Param        id  path  string  true  "Transaction ID"
// @Success      200  {object}  utils.ApiResponse
// @Failure      401  {object}  utils.ApiResponse
// @Security     BearerAuth
// @Router       /transactions/{id} [delete]
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

// GetManyTransactionHandler godoc
// @Summary      List transactions
// @Tags         transactions
// @Produce      json
// @Param        user_id     query  string  false  "Filter by user ID (admin only)"
// @Param        page        query  int     false  "Page number"
// @Param        page_count  query  int     false  "Items per page"
// @Success      200  {object}  utils.ApiResponse
// @Failure      401  {object}  utils.ApiResponse
// @Security     BearerAuth
// @Router       /transactions [get]
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

// GetTransactionCashFlow godoc
// @Summary      Get cash flow over a date range
// @Tags         transactions
// @Produce      json
// @Param        from  query  string  false  "Start date (YYYY-MM-DD)"
// @Param        to    query  string  false  "End date (YYYY-MM-DD)"
// @Success      200   {object}  utils.ApiResponse
// @Failure      401   {object}  utils.ApiResponse
// @Security     BearerAuth
// @Router       /transactions/inflow [get]
func (th *TransactionHandler) GetTransactionCashFlow(w http.ResponseWriter, r *http.Request) {
	userID, ok := utils.GetUserIDFromContext(r.Context())
	if !ok {
		utils.WriteToJson(w, http.StatusUnauthorized, utils.ApiResponse{
			StatusCode: http.StatusUnauthorized,
			Message:    "unauthorized",
		})
		return
	}

	query := TransactionCashFlowQuery{
		From: r.URL.Query().Get("from"),
		To:   r.URL.Query().Get("to"),
	}

	response := th.service.GetTransactionCashFlow(r.Context(), userID, query)
	utils.WriteToJson(w, response.StatusCode, response)
}
