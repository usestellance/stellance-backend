package admin

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/The-True-Hooha/stellance-backend/pkg/utils"
)

func adminID(r *http.Request) string {
	id, _ := utils.GetUserIDFromContext(r.Context())
	return id
}

func clientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return ip
	}
	return r.RemoteAddr
}

type AdminHandler struct {
	service *AdminService
}

func NewAdminHandler(s *AdminService) *AdminHandler {
	return &AdminHandler{service: s}
}

// GetStats godoc
// @Summary      Get platform statistics
// @Description  Platform-wide totals: users, invoices, paid and overdue invoice counts and total revenue. Admin only.
// @Tags         admin
// @Produce      json
// @Success      200  {object}  utils.ApiResponse{data=AdminStats}  "Platform statistics"
// @Failure      401  {object}  utils.ApiResponse  "Missing or invalid bearer token"
// @Failure      403  {object}  utils.ApiResponse  "Caller is not an admin"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
// @Security     BearerAuth
// @Router       /admin/stats [get]
func (h *AdminHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	resp := h.service.GetStats(r.Context())
	utils.WriteToJson(w, resp.StatusCode, resp)
}

// ListUsers godoc
// @Summary      List all users
// @Description  Paginated list of all users, newest first. `search` matches email and name.
// @Tags         admin
// @Produce      json
// @Param        page    query  int     false  "Page number"
// @Param        limit   query  int     false  "Items per page"
// @Param        search  query  string  false  "Search by email or name"
// @Success      200  {object}  utils.ApiResponse{data=AdminUserList}  "Users page"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      401  {object}  utils.ApiResponse  "Missing or invalid bearer token"
// @Failure      403  {object}  utils.ApiResponse  "Caller is not an admin"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
// @Security     BearerAuth
// @Router       /admin/users [get]
func (h *AdminHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	search := r.URL.Query().Get("search")
	resp := h.service.ListUsers(r.Context(), page, limit, search)
	utils.WriteToJson(w, resp.StatusCode, resp)
}

// GetUser godoc
// @Summary      Get user by ID
// @Description  User record plus wallets, the 10 most recent transactions and the 20 most recent activity entries. Empty collections are returned as `[]`, never `null`.
// @Tags         admin
// @Produce      json
// @Param        id   path  string  true  "User ID"
// @Success      200  {object}  utils.ApiResponse{data=AdminUserDetail}  "User detail"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      401  {object}  utils.ApiResponse  "Missing or invalid bearer token"
// @Failure      403  {object}  utils.ApiResponse  "Caller is not an admin"
// @Failure      404  {object}  utils.ApiResponse  "Not found"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
// @Security     BearerAuth
// @Router       /admin/users/{id} [get]
func (h *AdminHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{StatusCode: http.StatusBadRequest, Message: "user id required"})
		return
	}
	resp := h.service.GetUser(r.Context(), id)
	utils.WriteToJson(w, resp.StatusCode, resp)
}

// SetUserStatus godoc
// @Summary      Set user active status
// @Description  Activate or deactivate a user. A deactivated user's existing tokens are rejected on the next request.
// @Tags         admin
// @Produce      json
// @Param        id      path   string  true  "User ID"
// @Param        active  query  bool    true  "true = activate, false = deactivate"
// @Success      200  {object}  utils.ApiResponse  "Status updated"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      401  {object}  utils.ApiResponse  "Missing or invalid bearer token"
// @Failure      403  {object}  utils.ApiResponse  "Caller is not an admin"
// @Failure      404  {object}  utils.ApiResponse  "Not found"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
// @Security     BearerAuth
// @Router       /admin/users/{id}/status [patch]
func (h *AdminHandler) SetUserStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{StatusCode: http.StatusBadRequest, Message: "user id required"})
		return
	}
	activeParam := r.URL.Query().Get("active")
	if activeParam == "" {
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{StatusCode: http.StatusBadRequest, Message: "active query param required (true or false)"})
		return
	}
	active, err := strconv.ParseBool(activeParam)
	if err != nil {
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{StatusCode: http.StatusBadRequest, Message: "active must be true or false"})
		return
	}
	resp := h.service.SetUserActive(r.Context(), id, active, adminID(r), clientIP(r))
	utils.WriteToJson(w, resp.StatusCode, resp)
}

// DeleteUser godoc
// @Summary      Delete user
// @Description  Permanently delete a user and the data that cascades from it. Users who own invoices cannot be deleted.
// @Tags         admin
// @Produce      json
// @Param        id   path  string  true  "User ID"
// @Success      200  {object}  utils.ApiResponse  "User deleted"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      401  {object}  utils.ApiResponse  "Missing or invalid bearer token"
// @Failure      403  {object}  utils.ApiResponse  "Caller is not an admin"
// @Failure      404  {object}  utils.ApiResponse  "Not found"
// @Failure      409  {object}  utils.ApiResponse  "User owns invoices"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
// @Security     BearerAuth
// @Router       /admin/users/{id} [delete]
func (h *AdminHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{StatusCode: http.StatusBadRequest, Message: "user id required"})
		return
	}
	resp := h.service.DeleteUser(r.Context(), id, adminID(r), clientIP(r))
	utils.WriteToJson(w, resp.StatusCode, resp)
}

// ListInvoices godoc
// @Summary      List all invoices
// @Description  Paginated list of every invoice on the platform, filterable by status and search term.
// @Tags         admin
// @Produce      json
// @Param        page    query  int     false  "Page number"
// @Param        limit   query  int     false  "Items per page"
// @Param        status  query  string  false  "Filter by status (draft, paid, overdue, cancelled)"
// @Param        search  query  string  false  "Search by invoice number or email"
// @Success      200  {object}  utils.ApiResponse{data=AdminInvoiceList}  "Invoices page"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      401  {object}  utils.ApiResponse  "Missing or invalid bearer token"
// @Failure      403  {object}  utils.ApiResponse  "Caller is not an admin"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
// @Security     BearerAuth
// @Router       /admin/invoices [get]
func (h *AdminHandler) ListInvoices(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	status := r.URL.Query().Get("status")
	search := r.URL.Query().Get("search")
	resp := h.service.ListInvoices(r.Context(), page, limit, status, search)
	utils.WriteToJson(w, resp.StatusCode, resp)
}

// GetInvoice godoc
// @Summary      Get invoice by ID (admin)
// @Description  One invoice with its creator, regardless of owner.
// @Tags         admin
// @Produce      json
// @Param        id   path  string  true  "Invoice ID"
// @Success      200  {object}  utils.ApiResponse{data=AdminInvoiceRow}  "Invoice"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      401  {object}  utils.ApiResponse  "Missing or invalid bearer token"
// @Failure      403  {object}  utils.ApiResponse  "Caller is not an admin"
// @Failure      404  {object}  utils.ApiResponse  "Not found"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
// @Security     BearerAuth
// @Router       /admin/invoices/{id} [get]
func (h *AdminHandler) GetInvoice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{StatusCode: http.StatusBadRequest, Message: "invoice id required"})
		return
	}
	resp := h.service.GetInvoice(r.Context(), id)
	utils.WriteToJson(w, resp.StatusCode, resp)
}

// ListTransactions godoc
// @Summary      List all transactions
// @Description  Paginated ledger of every transaction. `search` matches user email and invoice number.
// @Tags         admin
// @Produce      json
// @Param        page    query  int     false  "Page number"
// @Param        limit   query  int     false  "Items per page"
// @Param        search  query  string  false  "Search by user email or invoice number"
// @Success      200  {object}  utils.ApiResponse{data=AdminTransactionList}  "Transactions page"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      401  {object}  utils.ApiResponse  "Missing or invalid bearer token"
// @Failure      403  {object}  utils.ApiResponse  "Caller is not an admin"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
// @Security     BearerAuth
// @Router       /admin/transactions [get]
func (h *AdminHandler) ListTransactions(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	search := r.URL.Query().Get("search")
	resp := h.service.ListTransactions(r.Context(), page, limit, search)
	utils.WriteToJson(w, resp.StatusCode, resp)
}

// GetUserInvoices godoc
// @Summary      Get invoices for a user
// @Description  Paginated invoices created by one user.
// @Tags         admin
// @Produce      json
// @Param        id     path   string  true   "User ID"
// @Param        page   query  int     false  "Page number"
// @Param        limit  query  int     false  "Items per page"
// @Success      200  {object}  utils.ApiResponse{data=AdminInvoiceList}  "Invoices page"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      401  {object}  utils.ApiResponse  "Missing or invalid bearer token"
// @Failure      403  {object}  utils.ApiResponse  "Caller is not an admin"
// @Failure      404  {object}  utils.ApiResponse  "Not found"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
// @Security     BearerAuth
// @Router       /admin/users/{id}/invoices [get]
func (h *AdminHandler) GetUserInvoices(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{StatusCode: http.StatusBadRequest, Message: "user id required"})
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	resp := h.service.GetUserInvoices(r.Context(), id, page, limit)
	utils.WriteToJson(w, resp.StatusCode, resp)
}

// GetUserTransactions godoc
// @Summary      Get transactions for a user
// @Description  Paginated ledger entries belonging to one user.
// @Tags         admin
// @Produce      json
// @Param        id     path   string  true   "User ID"
// @Param        page   query  int     false  "Page number"
// @Param        limit  query  int     false  "Items per page"
// @Success      200  {object}  utils.ApiResponse{data=AdminTransactionList}  "Transactions page"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      401  {object}  utils.ApiResponse  "Missing or invalid bearer token"
// @Failure      403  {object}  utils.ApiResponse  "Caller is not an admin"
// @Failure      404  {object}  utils.ApiResponse  "Not found"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
// @Security     BearerAuth
// @Router       /admin/users/{id}/transactions [get]
func (h *AdminHandler) GetUserTransactions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{StatusCode: http.StatusBadRequest, Message: "user id required"})
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	resp := h.service.GetUserTransactions(r.Context(), id, page, limit)
	utils.WriteToJson(w, resp.StatusCode, resp)
}

// GetUserActivity godoc
// @Summary      Get user activity log
// @Description  Paginated audit trail of one user's actions (logins, invoice and wallet events).
// @Tags         admin
// @Produce      json
// @Param        id     path   string  true   "User ID"
// @Param        page   query  int     false  "Page number"
// @Param        limit  query  int     false  "Items per page"
// @Success      200  {object}  utils.ApiResponse{data=AdminActivityList}  "Activity page"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      401  {object}  utils.ApiResponse  "Missing or invalid bearer token"
// @Failure      403  {object}  utils.ApiResponse  "Caller is not an admin"
// @Failure      404  {object}  utils.ApiResponse  "Not found"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
// @Security     BearerAuth
// @Router       /admin/users/{id}/activity [get]
func (h *AdminHandler) GetUserActivity(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{StatusCode: http.StatusBadRequest, Message: "user id required"})
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	resp := h.service.GetUserActivity(r.Context(), id, page, limit)
	utils.WriteToJson(w, resp.StatusCode, resp)
}

// AdminResetUserPassword godoc
// @Summary      Admin-triggered password reset
// @Description  Send the user a password reset email. The admin never sees the new password.
// @Tags         admin
// @Produce      json
// @Param        id  path  string  true  "User ID"
// @Success      200  {object}  utils.ApiResponse  "Reset email sent"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      401  {object}  utils.ApiResponse  "Missing or invalid bearer token"
// @Failure      403  {object}  utils.ApiResponse  "Caller is not an admin"
// @Failure      404  {object}  utils.ApiResponse  "Not found"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
// @Security     BearerAuth
// @Router       /admin/users/{id}/reset-password [post]
func (h *AdminHandler) AdminResetUserPassword(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{StatusCode: http.StatusBadRequest, Message: "user id required"})
		return
	}
	resp := h.service.AdminResetUserPassword(r.Context(), id, adminID(r), clientIP(r))
	utils.WriteToJson(w, resp.StatusCode, resp)
}

// GetStellarNetwork godoc
// @Summary      Get current Stellar network stage
// @Description  Stellar network (testnet or mainnet) used by payment detection and wallet operations. Seeded as `testnet` at server start when unset.
// @Tags         admin
// @Produce      json
// @Success      200  {object}  utils.ApiResponse{data=NetworkConfig}  "Current network"
// @Failure      401  {object}  utils.ApiResponse  "Missing or invalid bearer token"
// @Failure      403  {object}  utils.ApiResponse  "Caller is not an admin"
// @Failure      404  {object}  utils.ApiResponse  "Network config not set"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
// @Security     BearerAuth
// @Router       /admin/config/network [get]
func (h *AdminHandler) GetStellarNetwork(w http.ResponseWriter, r *http.Request) {
	resp := h.service.GetStellarNetwork(r.Context())
	utils.WriteToJson(w, resp.StatusCode, resp)
}

// SetStellarNetwork godoc
// @Summary      Switch Stellar network stage
// @Description  Switch the Stellar network. Payment cursors are kept per network, so switching does not replay history.
// @Tags         admin
// @Accept       json
// @Produce      json
// @Param        body  body  NetworkUpdate  true  "Target network"
// @Success      200  {object}  utils.ApiResponse  "Network updated"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      401  {object}  utils.ApiResponse  "Missing or invalid bearer token"
// @Failure      403  {object}  utils.ApiResponse  "Caller is not an admin"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
// @Security     BearerAuth
// @Router       /admin/config/network [patch]
func (h *AdminHandler) SetStellarNetwork(w http.ResponseWriter, r *http.Request) {
	var body NetworkUpdate
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Stage == "" {
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{StatusCode: http.StatusBadRequest, Message: "stage is required"})
		return
	}
	resp := h.service.SetStellarNetwork(r.Context(), body.Stage, adminID(r))
	utils.WriteToJson(w, resp.StatusCode, resp)
}
