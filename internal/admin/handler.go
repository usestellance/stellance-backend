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
// @Description  Returns total users, invoices, revenue, and overdue counts
// @Tags         admin
// @Produce      json
// @Success      200  {object}  utils.ApiResponse{data=AdminStats}
// @Failure      403  {object}  utils.ApiResponse
// @Security     BearerAuth
// @Router       /admin/stats [get]
func (h *AdminHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	resp := h.service.GetStats(r.Context())
	utils.WriteToJson(w, resp.StatusCode, resp)
}

// ListUsers godoc
// @Summary      List all users
// @Description  Paginated list of users with optional search
// @Tags         admin
// @Produce      json
// @Param        page    query  int     false  "Page number"
// @Param        limit   query  int     false  "Items per page"
// @Param        search  query  string  false  "Search by email or name"
// @Success      200  {object}  utils.ApiResponse
// @Failure      403  {object}  utils.ApiResponse
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
// @Description  Returns full user details for admin view
// @Tags         admin
// @Produce      json
// @Param        id   path  string  true  "User ID"
// @Success      200  {object}  utils.ApiResponse{data=AdminUserRow}
// @Failure      404  {object}  utils.ApiResponse
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

// ActivateUser godoc
// @Summary      Activate user account
// @Description  Sets user is_active = true
// @Tags         admin
// @Produce      json
// @Param        id   path  string  true  "User ID"
// @Success      200  {object}  utils.ApiResponse
// @Failure      404  {object}  utils.ApiResponse
// @Security     BearerAuth
// @Router       /admin/users/{id}/activate [patch]
func (h *AdminHandler) ActivateUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{StatusCode: http.StatusBadRequest, Message: "user id required"})
		return
	}
	resp := h.service.SetUserActive(r.Context(), id, true, adminID(r), clientIP(r))
	utils.WriteToJson(w, resp.StatusCode, resp)
}

// DeactivateUser godoc
// @Summary      Deactivate user account
// @Description  Sets user is_active = false
// @Tags         admin
// @Produce      json
// @Param        id   path  string  true  "User ID"
// @Success      200  {object}  utils.ApiResponse
// @Failure      404  {object}  utils.ApiResponse
// @Security     BearerAuth
// @Router       /admin/users/{id}/deactivate [patch]
func (h *AdminHandler) DeactivateUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{StatusCode: http.StatusBadRequest, Message: "user id required"})
		return
	}
	resp := h.service.SetUserActive(r.Context(), id, false, adminID(r), clientIP(r))
	utils.WriteToJson(w, resp.StatusCode, resp)
}

// DeleteUser godoc
// @Summary      Delete user
// @Description  Permanently deletes a user account
// @Tags         admin
// @Produce      json
// @Param        id   path  string  true  "User ID"
// @Success      200  {object}  utils.ApiResponse
// @Failure      404  {object}  utils.ApiResponse
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
// @Description  Paginated list of invoices with optional status and search filters
// @Tags         admin
// @Produce      json
// @Param        page    query  int     false  "Page number"
// @Param        limit   query  int     false  "Items per page"
// @Param        status  query  string  false  "Filter by status (draft, paid, overdue, cancelled)"
// @Param        search  query  string  false  "Search by invoice number or email"
// @Success      200  {object}  utils.ApiResponse
// @Failure      403  {object}  utils.ApiResponse
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
// @Description  Returns invoice details for admin view
// @Tags         admin
// @Produce      json
// @Param        id   path  string  true  "Invoice ID"
// @Success      200  {object}  utils.ApiResponse{data=AdminInvoiceRow}
// @Failure      404  {object}  utils.ApiResponse
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
// @Description  Paginated list of all platform transactions
// @Tags         admin
// @Produce      json
// @Param        page    query  int     false  "Page number"
// @Param        limit   query  int     false  "Items per page"
// @Param        search  query  string  false  "Search by user email or invoice number"
// @Success      200  {object}  utils.ApiResponse
// @Failure      403  {object}  utils.ApiResponse
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
// @Description  Returns paginated invoices created by a specific user
// @Tags         admin
// @Produce      json
// @Param        id     path   string  true   "User ID"
// @Param        page   query  int     false  "Page number"
// @Param        limit  query  int     false  "Items per page"
// @Success      200  {object}  utils.ApiResponse
// @Failure      404  {object}  utils.ApiResponse
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
// @Description  Returns paginated transactions belonging to a specific user
// @Tags         admin
// @Produce      json
// @Param        id     path   string  true   "User ID"
// @Param        page   query  int     false  "Page number"
// @Param        limit  query  int     false  "Items per page"
// @Success      200  {object}  utils.ApiResponse
// @Failure      404  {object}  utils.ApiResponse
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

// SuspendUser godoc
// @Summary      Toggle user suspension
// @Description  Toggles user is_active — suspends active users, unsuspends suspended ones
// @Tags         admin
// @Produce      json
// @Param        id  path  string  true  "User ID"
// @Success      200  {object}  utils.ApiResponse
// @Failure      404  {object}  utils.ApiResponse
// @Security     BearerAuth
// @Router       /admin/users/{id}/suspend [patch]
func (h *AdminHandler) SuspendUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{StatusCode: http.StatusBadRequest, Message: "user id required"})
		return
	}
	resp := h.service.SuspendUser(r.Context(), id, adminID(r), clientIP(r))
	utils.WriteToJson(w, resp.StatusCode, resp)
}

// GetUserActivity godoc
// @Summary      Get user activity log
// @Description  Returns paginated activity log entries for a specific user
// @Tags         admin
// @Produce      json
// @Param        id     path   string  true   "User ID"
// @Param        page   query  int     false  "Page number"
// @Param        limit  query  int     false  "Items per page"
// @Success      200  {object}  utils.ApiResponse
// @Failure      404  {object}  utils.ApiResponse
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
// @Description  Generates OTP and sends password reset email to the user
// @Tags         admin
// @Produce      json
// @Param        id  path  string  true  "User ID"
// @Success      200  {object}  utils.ApiResponse
// @Failure      404  {object}  utils.ApiResponse
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
// @Description  Returns whether the platform is running on testnet or mainnet
// @Tags         admin
// @Produce      json
// @Success      200  {object}  utils.ApiResponse
// @Security     BearerAuth
// @Router       /admin/config/network [get]
func (h *AdminHandler) GetStellarNetwork(w http.ResponseWriter, r *http.Request) {
	resp := h.service.GetStellarNetwork(r.Context())
	utils.WriteToJson(w, resp.StatusCode, resp)
}

// SetStellarNetwork godoc
// @Summary      Switch Stellar network stage
// @Description  Switches platform between testnet and mainnet. Value stored encrypted.
// @Tags         admin
// @Accept       json
// @Produce      json
// @Param        body  body  object  true  "stage: testnet or mainnet"
// @Success      200  {object}  utils.ApiResponse
// @Failure      400  {object}  utils.ApiResponse
// @Security     BearerAuth
// @Router       /admin/config/network [patch]
func (h *AdminHandler) SetStellarNetwork(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Stage string `json:"stage"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Stage == "" {
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{StatusCode: http.StatusBadRequest, Message: "stage is required"})
		return
	}
	resp := h.service.SetStellarNetwork(r.Context(), body.Stage, adminID(r))
	utils.WriteToJson(w, resp.StatusCode, resp)
}
