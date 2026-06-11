package wallet

import (
	"encoding/json"
	"fmt"
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
// @Tags         wallet
// @Produce      json
// @Success      201  {object}  utils.ApiResponse
// @Failure      401  {object}  utils.ApiResponse
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
// @Tags         wallet
// @Produce      json
// @Param        id  path  string  true  "Wallet ID"
// @Success      200  {object}  utils.ApiResponse
// @Failure      401  {object}  utils.ApiResponse
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
// @Tags         wallet
// @Produce      json
// @Param        id  path  string  true  "Wallet ID"
// @Success      200  {object}  utils.ApiResponse
// @Failure      401  {object}  utils.ApiResponse
// @Security     BearerAuth
// @Router       /wallet/{id}/export [get]
func (h *WalletHandler) ExportWalletHandler(w http.ResponseWriter, r *http.Request) {
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
	fmt.Println(walletID, "checking the wallet ID")

	wallet := h.service.ExportWalletKeys(ctx, walletID, id, user.UserRole(role))
	utils.WriteToJson(w, wallet.StatusCode, wallet)
}

// LookupWalletHandler godoc
// @Summary      Lookup wallet address by email
// @Description  Returns the primary wallet address for a given user email. Used to resolve a recipient before sending payment.
// @Tags         wallet
// @Produce      json
// @Param        email  query  string  true  "Recipient email address"
// @Success      200    {object}  utils.ApiResponse
// @Failure      404    {object}  utils.ApiResponse
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
// @Description  Sets a 4-8 digit numeric PIN required for all payment operations
// @Tags         wallet
// @Accept       json
// @Produce      json
// @Param        id    path  string     true  "Wallet ID"
// @Param        body  body  SetPinDTO  true  "PIN"
// @Success      200  {object}  utils.ApiResponse
// @Failure      400  {object}  utils.ApiResponse
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
// @Description  Pays invoice using PathPaymentStrictReceive — payer sends source asset, vendor receives exact USDC
// @Tags         wallet
// @Accept       json
// @Produce      json
// @Param        id    path  string         true  "Wallet ID"
// @Param        body  body  PayInvoiceDTO  true  "Payment details"
// @Success      200  {object}  utils.ApiResponse{data=PathPaymentResult}
// @Failure      400  {object}  utils.ApiResponse
// @Failure      403  {object}  utils.ApiResponse
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
// @Description  Sends assets to any Stellar address. Uses path payment if source and dest assets differ
// @Tags         wallet
// @Accept       json
// @Produce      json
// @Param        id    path  string       true  "Wallet ID"
// @Param        body  body  TransferDTO  true  "Transfer details"
// @Success      200  {object}  utils.ApiResponse{data=PathPaymentResult}
// @Failure      400  {object}  utils.ApiResponse
// @Failure      403  {object}  utils.ApiResponse
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
