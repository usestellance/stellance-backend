package admin

import (
	"net/http"
	"strconv"

	"github.com/The-True-Hooha/stellance-backend/pkg/utils"
)

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
	resp := h.service.SetUserActive(r.Context(), id, true)
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
	resp := h.service.SetUserActive(r.Context(), id, false)
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
	resp := h.service.DeleteUser(r.Context(), id)
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
