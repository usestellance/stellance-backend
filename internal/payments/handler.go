package payments

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/The-True-Hooha/stellance-backend/pkg/utils"
	"github.com/go-playground/validator/v10"
)

type PaymentHandler struct {
	service   *PaymentService
	validator *validator.Validate
}

func NewPaymentHandler(s *PaymentService) *PaymentHandler {
	return &PaymentHandler{service: s, validator: validator.New()}
}

func write(w http.ResponseWriter, resp *utils.ApiResponse) {
	utils.WriteToJson(w, resp.StatusCode, resp)
}

func userID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id, ok := utils.GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}
	return id, ok
}

// GetInvoicePaymentsHandler godoc
// @Summary      Payment request, status and payment records of an invoice (v2)
// @Description  Owner view of one invoice's payment request: recipient, memo reference, expected amount and asset, derived `payment_status`, and every payment recorded against it. Fixes the recipient to the creator's primary wallet on first use.
// @Tags         payments-v2
// @Produce      json
// @Param        id   path  string  true  "Invoice ID"
// @Success      200  {object}  utils.ApiResponse{data=PaymentRequest}  "Payment request with records"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      401  {object}  utils.ApiResponse  "Missing or invalid bearer token"
// @Failure      403  {object}  utils.ApiResponse  "Not the owner"
// @Failure      404  {object}  utils.ApiResponse  "Invoice not found"
// @Failure      409  {object}  utils.ApiResponse  "Creator has no active Stellar wallet"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
// @Security     BearerAuth
// @Router       /payments/invoice/{id} [get]
func (h *PaymentHandler) GetInvoicePaymentsHandler(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	write(w, h.service.GetInvoicePayments(r.Context(), uid, r.PathValue("id")))
}

// GetPublicRequestHandler godoc
// @Summary      Payment instructions for an invoice, by invoice url (v2)
// @Description  Public payer view by invoice short url: what to send, to whom and with which memo. Draft invoices return 404. See docs/payment-status-model.md for status values.
// @Tags         payments-v2
// @Produce      json
// @Param        invoice_url  path  string  true  "Invoice URL"
// @Success      200  {object}  utils.ApiResponse{data=PaymentRequest}  "Payment request"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      404  {object}  utils.ApiResponse  "Invoice not found or still a draft"
// @Failure      409  {object}  utils.ApiResponse  "Invoice cannot accept payment yet"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
// @Router       /payments/request/{invoice_url} [get]
func (h *PaymentHandler) GetPublicRequestHandler(w http.ResponseWriter, r *http.Request) {
	write(w, h.service.GetPublicRequest(r.Context(), r.PathValue("invoice_url")))
}

// ListPaymentsHandler godoc
// @Summary      Payments received on the user's wallets (verified, invalid, unmatched, failed) (v2)
// @Description  Payments received on the signed-in user's wallets, newest first: verified, invalid, unmatched and failed. Filter with `status`. `limit` above 100 falls back to 20.
// @Tags         payments-v2
// @Produce      json
// @Param        status  query  string  false  "verification status"
// @Param        page    query  int     false  "page number, from 1"
// @Param        limit   query  int     false  "page size, default 20, max 100"
// @Success      200  {object}  utils.ApiResponse{data=PaymentList}  "Payments page"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      401  {object}  utils.ApiResponse  "Missing or invalid bearer token"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
// @Security     BearerAuth
// @Router       /payments [get]
func (h *PaymentHandler) ListPaymentsHandler(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	status := r.URL.Query().Get("status")
	switch VerificationStatus(status) {
	case "", StatusPending, StatusDetected, StatusVerified, StatusInvalid, StatusUnmatched, StatusFailed:
	default:
		write(w, &utils.ApiResponse{StatusCode: http.StatusBadRequest, Message: "invalid status filter"})
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	write(w, h.service.ListPayments(r.Context(), uid, status, page, limit))
}

// VerifyTransactionHandler godoc
// @Summary      Re-check a Stellar transaction against invoices
// @Description  Re-check a Stellar transaction hash against invoices. The caller never supplies the outcome: the chain is read and every payment operation is evaluated. Only payments on the caller's own wallets are returned. 404 when the hash is unknown to Stellar; 502 only when Stellar is unreachable.
// @Tags         payments-v2
// @Accept       json
// @Produce      json
// @Param        body  body  VerifyTransactionDTO  true  "Transaction hash"
// @Success      200  {object}  utils.ApiResponse{data=[]PaymentRecord}  "Payments found in the transaction"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      401  {object}  utils.ApiResponse  "Missing or invalid bearer token"
// @Failure      404  {object}  utils.ApiResponse  "Transaction not found on Stellar"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
// @Failure      502  {object}  utils.ApiResponse  "Stellar unreachable"
// @Security     BearerAuth
// @Router       /payments/verify [post]
func (h *PaymentHandler) VerifyTransactionHandler(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	var dto VerifyTransactionDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		write(w, &utils.ApiResponse{StatusCode: http.StatusBadRequest, Message: "invalid request body"})
		return
	}
	if err := h.validator.Struct(dto); err != nil {
		utils.HandleValidationError(w, err)
		return
	}
	write(w, h.service.VerifyTransactionForUser(r.Context(), uid, dto.TransactionHash))
}

// AdminListPaymentsHandler godoc
// @Summary      All platform payments, with merchant email (v2)
// @Description  Every detected payment on the platform, newest first, including unmatched, invalid and failed ones. `search` matches transaction hash, payment reference, sender or recipient address and invoice number. Records include `merchant_email`.
// @Tags         admin
// @Produce      json
// @Param        page    query  int     false  "Page number"
// @Param        limit   query  int     false  "Items per page, max 100"
// @Param        status  query  string  false  "verification status"
// @Param        search  query  string  false  "transaction hash, payment reference, address or invoice number"
// @Success      200  {object}  utils.ApiResponse{data=PaymentList}  "Payments page"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      401  {object}  utils.ApiResponse  "Missing or invalid bearer token"
// @Failure      403  {object}  utils.ApiResponse  "Caller is not an admin"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
// @Security     BearerAuth
// @Router       /admin/payments [get]
func (h *PaymentHandler) AdminListPaymentsHandler(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	switch VerificationStatus(status) {
	case "", StatusPending, StatusDetected, StatusVerified, StatusInvalid, StatusUnmatched, StatusFailed:
	default:
		write(w, &utils.ApiResponse{StatusCode: http.StatusBadRequest, Message: "invalid status filter"})
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	write(w, h.service.AdminListPayments(r.Context(), status, r.URL.Query().Get("search"), page, limit))
}

// GetPaymentHandler godoc
// @Summary      One payment received on the user's wallets (v2)
// @Description  One payment received on the signed-in user's wallets, with `failure_code`, per-check results (`checks`) and the invoice's `expected` values. 404 for payments on other users' wallets.
// @Tags         payments-v2
// @Produce      json
// @Param        id   path  string  true  "Payment ID"
// @Success      200  {object}  utils.ApiResponse{data=PaymentRecord}  "Payment"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      401  {object}  utils.ApiResponse  "Missing or invalid bearer token"
// @Failure      404  {object}  utils.ApiResponse  "Payment not found"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
// @Security     BearerAuth
// @Router       /payments/{id} [get]
func (h *PaymentHandler) GetPaymentHandler(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	write(w, h.service.GetPayment(r.Context(), uid, r.PathValue("id")))
}
