package wallet

import (
	"net/http"

	"github.com/The-True-Hooha/stellance-backend/internal/middleware"
	"github.com/The-True-Hooha/stellance-backend/pkg/httpx"
)

func RegisterWalletRoutes(apiV1 *httpx.RouteGroup, router *http.ServeMux, walletService *WalletService) {
	walletHandler := NewWalletHandler(walletService)

	authMiddleware := middleware.NewAuthMiddleware(walletService.jwt)

	walletGroup := apiV1.AddGroup("/wallet")

	walletGroup.HandleFunc("GET /lookup", authMiddleware.Authenticate(http.HandlerFunc(walletHandler.LookupWalletHandler)).ServeHTTP)
	walletGroup.HandleFunc("POST /", authMiddleware.Authenticate(http.HandlerFunc(walletHandler.CreateWalletHandler)).ServeHTTP)
	walletGroup.HandleFunc("GET /{id}", authMiddleware.Authenticate(http.HandlerFunc(walletHandler.GetWalletHandler)).ServeHTTP)
	walletGroup.HandleFunc("GET /export/{id}", authMiddleware.Authenticate(http.HandlerFunc(walletHandler.ExportWalletHandler)).ServeHTTP)
	walletGroup.HandleFunc("POST /{id}/pin", authMiddleware.Authenticate(http.HandlerFunc(walletHandler.SetPinHandler)).ServeHTTP)
	walletGroup.HandleFunc("POST /{id}/pay/invoice", authMiddleware.Authenticate(http.HandlerFunc(walletHandler.PayInvoiceHandler)).ServeHTTP)
	walletGroup.HandleFunc("POST /{id}/pay/transfer", authMiddleware.Authenticate(http.HandlerFunc(walletHandler.TransferHandler)).ServeHTTP)
}
