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
	invoiceGroup.HandleFunc("GET /search", authMiddleware.IsPublicAccess(http.HandlerFunc(invoiceHandler.GetInvoiceSearchHandler)).ServeHTTP)
	invoiceGroup.HandleFunc("GET /review/{id}", authMiddleware.IsPublicAccess(http.HandlerFunc(invoiceHandler.ReviewInvoiceHandler)).ServeHTTP)
	invoiceGroup.HandleFunc("POST /", authMiddleware.Authenticate(http.HandlerFunc(invoiceHandler.CreateNewInvoiceHandler)).ServeHTTP)
	invoiceGroup.HandleFunc("GET /", authMiddleware.Authenticate(http.HandlerFunc(invoiceHandler.GetManyInvoiceHandler)).ServeHTTP)
	invoiceGroup.HandleFunc("GET /{id}", authMiddleware.Authenticate(http.HandlerFunc(invoiceHandler.GetInvoiceByIDHandler)).ServeHTTP)
	invoiceGroup.HandleFunc("DELETE /{id}", authMiddleware.Authenticate(http.HandlerFunc(invoiceHandler.DeleteInvoiceHandler)).ServeHTTP)
	invoiceGroup.HandleFunc("POST /{id}", authMiddleware.Authenticate(http.HandlerFunc(invoiceHandler.EditInvoiceHandler)).ServeHTTP)
	invoiceGroup.HandleFunc("GET /send/{id}", authMiddleware.Authenticate(http.HandlerFunc(invoiceHandler.SendInvoice)).ServeHTTP)

}
