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
// @Description  Invoice amounts per bucket, in integer cents: total, pending (draft, sent, viewed, pending, payment_detected), paid, overdue, expired and invalid_payment.
// @Tags         transactions
// @Produce      json
// @Success      200  {object}  utils.ApiResponse{data=GetTransactionOverViewResponse}  "Amounts in cents"
// @Failure      401  {object}  utils.ApiResponse  "Missing or invalid bearer token"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
// @Security     BearerAuth
// @Router       /transaction/stats [get]
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
// @Description  One ledger entry owned by the caller.
// @Tags         transactions
// @Produce      json
// @Param        id  path  string  true  "Transaction ID"
// @Success      200  {object}  utils.ApiResponse{data=GetTransactionDto}  "Transaction"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      401  {object}  utils.ApiResponse  "Missing or invalid bearer token"
// @Failure      404  {object}  utils.ApiResponse  "Not found"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
// @Security     BearerAuth
// @Router       /transaction/id/{id} [get]
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
// @Description  Delete a ledger entry owned by the caller.
// @Tags         transactions
// @Produce      json
// @Param        id  path  string  true  "Transaction ID"
// @Success      200  {object}  utils.ApiResponse  "Deleted"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      401  {object}  utils.ApiResponse  "Missing or invalid bearer token"
// @Failure      404  {object}  utils.ApiResponse  "Not found"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
// @Security     BearerAuth
// @Router       /transaction/{id} [delete]
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
// @Description  Paginated ledger of the signed-in user: payments received, funding and outgoing transfers. `status` is the on-chain result; verification results live on /payments.
// @Tags         transactions
// @Produce      json
// @Param        user_id     query  string  false  "Filter by user ID (admin only)"
// @Param        page        query  int     false  "Page number, from 1 (default 1)"
// @Param        limit       query  int     false  "Items per page, default 10 (page_count is a deprecated alias)"
// @Param        type        query  string  false  "Ledger type" Enums(payment, withdrawal, funding, path_payment)
// @Param        status      query  string  false  "On-chain status" Enums(pending, confirmed, failed, aborted, refunded)
// @Param        from        query  string  false  "Created on or after this date, YYYY-MM-DD"
// @Param        to          query  string  false  "Created on or before this date, YYYY-MM-DD"
// @Success      200  {object}  utils.ApiResponse{data=PaginatedTransactions}  "Transactions page"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      401  {object}  utils.ApiResponse  "Missing or invalid bearer token"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
// @Security     BearerAuth
// @Router       /transaction [get]
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
		Type:   r.URL.Query().Get("type"),
		Status: r.URL.Query().Get("status"),
		From:   r.URL.Query().Get("from"),
		To:     r.URL.Query().Get("to"),
	}

	page := r.URL.Query().Get("page")
	if page != "" {
		if p, err := strconv.Atoi(page); err == nil {
			dto.Page = p
		}
	} else {
		dto.Page = 1
	}

	if ps, ok := utils.PageSizeParam(r, "page_count"); ok {
		dto.Count = ps
	} else {
		dto.Count = 10
	}

	log.Info("fetching transactions",
		"user_id", userID,
		"filters", dto,
	)

	response := handler.service.GetTransactionsPaginated(ctx, dto.Page, dto.Count, dto)
	utils.WriteToJson(w, response.StatusCode, response)
}

// CheckPaymentStatus godoc
// @Summary      Check payment status
// @Description  Look up the on-chain status of a ledger entry and its explorer link.
// @Tags         transactions
// @Produce      json
// @Param        id  path  string  true  "Transaction ID"
// @Success      200  {object}  utils.ApiResponse{data=PaymentStatusResult}  "Status"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      401  {object}  utils.ApiResponse  "Missing or invalid bearer token"
// @Failure      404  {object}  utils.ApiResponse  "Not found"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
// @Security     BearerAuth
// @Router       /transaction/status/{id} [get]
func (h *TransactionHandler) CheckPaymentStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := utils.GetUserIDFromContext(r.Context())
	if !ok {
		utils.WriteToJson(w, http.StatusUnauthorized, utils.ApiResponse{StatusCode: http.StatusUnauthorized, Message: "unauthorized"})
		return
	}
	id := r.PathValue("id")
	resp := h.service.CheckPaymentStatus(r.Context(), id, userID)
	utils.WriteToJson(w, resp.StatusCode, resp)
}

// GetTransactionCashFlow godoc
// @Summary      Get cash flow over a date range
// @Description  Paid-invoice cash flow grouped by month between `from` and `to` (YYYY-MM-DD). Amounts are integer cents.
// @Tags         transactions
// @Produce      json
// @Param        from  query  string  false  "Start date (YYYY-MM-DD)"
// @Param        to    query  string  false  "End date (YYYY-MM-DD)"
// @Success      200  {object}  utils.ApiResponse{data=GetTransactionCashFlowResponse}  "Cash flow"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      401  {object}  utils.ApiResponse  "Missing or invalid bearer token"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
// @Security     BearerAuth
// @Router       /transaction/inflow [get]
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
