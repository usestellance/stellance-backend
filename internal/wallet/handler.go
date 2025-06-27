package wallet

import (
	"net/http"

	"github.com/The-True-Hooha/stellance-backend.git/internal/user"
	"github.com/The-True-Hooha/stellance-backend.git/pkg/utils"
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

func (h *WalletHandler) CreateWalletHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, ok := utils.GetUserIDFromContext(ctx)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}

	wallet := h.service.CreateWallet(ctx, id)
	utils.WriteToJson(w, wallet.StatusCode, wallet)
}

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

func (h *WalletHandler) ExportWalletHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := utils.GetUserIDFromContext(ctx)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}
	walletId := r.URL.Query().Get("wallet_id")

	role, ok := utils.GetRoleFromContext(ctx)
	if !ok {
		http.Error(w, "Unauthorized: missing role", http.StatusUnauthorized)
		return
	}

	wallet := h.service.ExportWalletKeys(ctx, walletId, id, user.UserRole(role))
	utils.WriteToJson(w, wallet.StatusCode, wallet)
}
