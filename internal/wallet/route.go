package wallet

import (
	"net/http"

	"github.com/The-True-Hooha/stellance-backend.git/internal/middleware"
	"github.com/The-True-Hooha/stellance-backend.git/pkg/httpx"
)

func RegisterWalletRoutes(apiV1 *httpx.RouteGroup, router *http.ServeMux, walletService *WalletService) {
	walletHandler := NewWalletHandler(walletService)

	authMiddleware := middleware.NewAuthMiddleware(walletService.jwt)

	walletGroup := apiV1.AddGroup("/wallet")

	walletGroup.HandleFunc("POST /", authMiddleware.Authenticate(http.HandlerFunc(walletHandler.CreateWalletHandler)).ServeHTTP)
	walletGroup.HandleFunc("GET /{id}", authMiddleware.Authenticate(http.HandlerFunc(walletHandler.GetWalletHandler)).ServeHTTP)
	walletGroup.HandleFunc("GET /", authMiddleware.Authenticate(http.HandlerFunc(walletHandler.ExportWalletHandler)).ServeHTTP)
}
