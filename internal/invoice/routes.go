package invoice

import (
	"net/http"

	"github.com/The-True-Hooha/stellance-backend.git/internal/middleware"
	"github.com/The-True-Hooha/stellance-backend.git/pkg/httpx"
)

func RegisterInvoiceRoutes(apiV1 *httpx.RouteGroup, router *http.ServeMux, invoiceService *InvoiceService) {
	invoiceHandler := NewInvoiceHandler(invoiceService)

	authMiddleware := middleware.NewAuthMiddleware(invoiceService.jwt)

	invoiceGroup := apiV1.AddGroup("/invoice")
	invoiceGroup.HandleFunc("POST /", authMiddleware.Authenticate(http.HandlerFunc(invoiceHandler.CreateNewInvoiceHandler)).ServeHTTP)
	invoiceGroup.HandleFunc("GET /", authMiddleware.Authenticate(http.HandlerFunc(invoiceHandler.GetManyInvoiceHandler)).ServeHTTP)
	invoiceGroup.HandleFunc("GET /search", authMiddleware.Authenticate(http.HandlerFunc(invoiceHandler.GetInvoiceByID)).ServeHTTP)
}
