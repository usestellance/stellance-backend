package wallet

import (
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
