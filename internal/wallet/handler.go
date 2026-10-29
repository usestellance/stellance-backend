package wallet

import (
	"encoding/json"
	"net/http"

	"github.com/The-True-Hooha/stellance-backend/internal/user"
	"github.com/The-True-Hooha/stellance-backend/pkg/utils"
	"github.com/go-playground/validator/v10"
)

type WalletHandler struct {
	service   *WalletService
	validator *validator.Validate
}

func NewWalletHandler(service *WalletService) *WalletHandler {
	return &WalletHandler{
		service:   service,
		validator: validator.New(),
	}
}

// CreateWalletHandler godoc
// @Summary      Create a Stellar wallet
// @Description  Create the user's Stellar wallet (one per user). On testnet the account is funded in the background. The secret key is stored encrypted.
// @Tags         wallet
// @Produce      json
// @Success      201  {object}  utils.ApiResponse{data=WalletResponseDto}  "Wallet created"
// @Failure      401  {object}  utils.ApiResponse  "Missing or invalid bearer token"
// @Failure      403  {object}  utils.ApiResponse  "Wallet limit reached: one wallet per user"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
// @Security     BearerAuth
// @Router       /wallet [post]
func (h *WalletHandler) CreateWalletHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, ok := utils.GetUserIDFromContext(ctx)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}

	wallet := h.service.CreateWallet(ctx, id)
	utils.WriteToJson(w, wallet.StatusCode, wallet)
}

// GetWalletHandler godoc
// @Summary      Get wallet by ID
// @Description  Wallet details with live balances.
// @Tags         wallet
// @Produce      json
// @Param        id  path  string  true  "Wallet ID"
// @Success      200  {object}  utils.ApiResponse{data=WalletResponseDto}  "Wallet"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      401  {object}  utils.ApiResponse  "Missing or invalid bearer token"
// @Failure      403  {object}  utils.ApiResponse  "Access denied: not your wallet"
// @Failure      404  {object}  utils.ApiResponse  "Not found"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
// @Security     BearerAuth
// @Router       /wallet/{id} [get]
func (h *WalletHandler) GetWalletHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := utils.GetUserIDFromContext(ctx)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}
	walletID := r.PathValue("id")

	role, ok := utils.GetRoleFromContext(ctx)
	if !ok {
		http.Error(w, "Unauthorized: missing role", http.StatusUnauthorized)
		return
	}

	wallet := h.service.GetUserWallet(ctx, id, walletID, user.UserRole(role))
	utils.WriteToJson(w, wallet.StatusCode, wallet)
}

// ExportWalletHandler godoc
// @Summary      Export wallet keys
// @Description  Return the wallet's secret key. Sensitive: the response must never be logged or cached.
// @Tags         wallet
// @Produce      json
// @Param        id  path  string  true  "Wallet ID"
// @Success      200  {object}  utils.ApiResponse{data=WalletExportResult}  "Secret key"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      401  {object}  utils.ApiResponse  "Missing or invalid bearer token"
// @Failure      403  {object}  utils.ApiResponse  "Not your wallet"
// @Failure      404  {object}  utils.ApiResponse  "Not found"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
// @Security     BearerAuth
// @Router       /wallet/export/{id} [get]
func (h *WalletHandler) ExportWalletHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := utils.GetUserIDFromContext(ctx)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	walletID := r.PathValue("id")

	role, ok := utils.GetRoleFromContext(ctx)
	if !ok {
		utils.WriteToJson(w, http.StatusUnauthorized, utils.ApiResponse{StatusCode: http.StatusUnauthorized, Message: "Unauthorized: missing role"})
		return
	}

	var body struct {
		Pin string `json:"pin"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	wallet := h.service.ExportWalletKeys(ctx, walletID, id, body.Pin, user.UserRole(role))
	utils.WriteToJson(w, wallet.StatusCode, wallet)
}

// LookupWalletHandler godoc
// @Summary      Lookup wallet address by email
// @Description  Find the primary wallet address of a user by email, for transfers.
// @Tags         wallet
// @Produce      json
// @Param        email  query  string  true  "Recipient email address"
// @Success      200  {object}  utils.ApiResponse{data=WalletLookupResult}  "Wallet address"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      401  {object}  utils.ApiResponse  "Missing or invalid bearer token"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
// @Security     BearerAuth
// @Router       /wallet/lookup [get]
func (h *WalletHandler) LookupWalletHandler(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	if email == "" {
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{StatusCode: http.StatusBadRequest, Message: "email is required"})
		return
	}
	resp := h.service.LookupWalletByEmail(r.Context(), email)
	utils.WriteToJson(w, resp.StatusCode, resp)
}

// SetPinHandler godoc
// @Summary      Set or update transaction PIN
// @Description  Set or replace the 4-8 digit PIN that authorises payments from this wallet.
// @Tags         wallet
// @Accept       json
// @Produce      json
// @Param        id    path  string     true  "Wallet ID"
// @Param        body  body  SetPinDTO  true  "PIN"
// @Success      200  {object}  utils.ApiResponse  "PIN set"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      401  {object}  utils.ApiResponse  "Missing or invalid bearer token"
// @Failure      403  {object}  utils.ApiResponse  "Not your wallet"
// @Failure      404  {object}  utils.ApiResponse  "Not found"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
// @Security     BearerAuth
// @Router       /wallet/{id}/pin [post]
func (h *WalletHandler) SetPinHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := utils.GetUserIDFromContext(r.Context())
	if !ok {
		utils.WriteToJson(w, http.StatusUnauthorized, utils.ApiResponse{StatusCode: http.StatusUnauthorized, Message: "unauthorized"})
		return
	}
	walletID := r.PathValue("id")
	var dto SetPinDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{StatusCode: http.StatusBadRequest, Message: "invalid request body"})
		return
	}
	if err := h.validator.Struct(dto); err != nil {
		utils.HandleValidationError(w, err)
		return
	}
	resp := h.service.SetPin(r.Context(), walletID, userID, dto.Pin)
	utils.WriteToJson(w, resp.StatusCode, resp)
}

// PayInvoiceHandler godoc
// @Summary      Pay an invoice via path payment
// @Description  Pay an invoice from this wallet, swapping XLM or USDC to the invoice asset by path payment. The memo is the invoice's payment reference; settlement happens only after the chain payment is verified, so `payment_status` may still be `detected`.
// @Tags         wallet
// @Accept       json
// @Produce      json
// @Param        id    path  string         true  "Wallet ID"
// @Param        body  body  PayInvoiceDTO  true  "Payment details"
// @Success      200  {object}  utils.ApiResponse{data=PathPaymentResult}  "Payment submitted"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      401  {object}  utils.ApiResponse  "Missing or invalid bearer token"
// @Failure      404  {object}  utils.ApiResponse  "Not found"
// @Failure      409  {object}  utils.ApiResponse  "Another payment is being verified, or the merchant has no wallet (error=wallet_required)"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
// @Failure      502  {object}  utils.ApiResponse  "Stellar rejected the transaction"
// @Security     BearerAuth
// @Router       /wallet/{id}/pay/invoice [post]
func (h *WalletHandler) PayInvoiceHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := utils.GetUserIDFromContext(r.Context())
	if !ok {
		utils.WriteToJson(w, http.StatusUnauthorized, utils.ApiResponse{StatusCode: http.StatusUnauthorized, Message: "unauthorized"})
		return
	}
	walletID := r.PathValue("id")
	var dto PayInvoiceDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{StatusCode: http.StatusBadRequest, Message: "invalid request body"})
		return
	}
	if err := h.validator.Struct(dto); err != nil {
		utils.HandleValidationError(w, err)
		return
	}
	resp := h.service.PayInvoice(r.Context(), walletID, userID, dto)
	utils.WriteToJson(w, resp.StatusCode, resp)
}

// TransferHandler godoc
// @Summary      Transfer funds to another wallet
// @Description  Transfer XLM or USDC to another address, converting between assets by path payment if needed.
// @Tags         wallet
// @Accept       json
// @Produce      json
// @Param        id    path  string       true  "Wallet ID"
// @Param        body  body  TransferDTO  true  "Transfer details"
// @Success      200  {object}  utils.ApiResponse{data=PathPaymentResult}  "Transfer submitted"
// @Failure      400  {object}  utils.ApiResponse  "Invalid request body or parameters"
// @Failure      401  {object}  utils.ApiResponse  "Missing or invalid bearer token"
// @Failure      404  {object}  utils.ApiResponse  "Not found"
// @Failure      500  {object}  utils.ApiResponse  "Unexpected server error"
// @Failure      502  {object}  utils.ApiResponse  "Stellar rejected the transaction"
// @Security     BearerAuth
// @Router       /wallet/{id}/pay/transfer [post]
func (h *WalletHandler) TransferHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := utils.GetUserIDFromContext(r.Context())
	if !ok {
		utils.WriteToJson(w, http.StatusUnauthorized, utils.ApiResponse{StatusCode: http.StatusUnauthorized, Message: "unauthorized"})
		return
	}
	walletID := r.PathValue("id")
	var dto TransferDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		utils.WriteToJson(w, http.StatusBadRequest, utils.ApiResponse{StatusCode: http.StatusBadRequest, Message: "invalid request body"})
		return
	}
	if err := h.validator.Struct(dto); err != nil {
		utils.HandleValidationError(w, err)
		return
	}
	resp := h.service.Transfer(r.Context(), walletID, userID, dto)
	utils.WriteToJson(w, resp.StatusCode, resp)
}
