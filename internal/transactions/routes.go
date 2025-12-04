package transactions

import (
	"net/http"

	"github.com/The-True-Hooha/stellance-backend/internal/middleware"
	"github.com/The-True-Hooha/stellance-backend/pkg/httpx"
)

func RegisterTransactionRoutes(apiV1 *httpx.RouteGroup, router *http.ServeMux, transactionService *TransactionService) {
	t := NewTransactionHandler(transactionService)

	authMiddleware := middleware.NewAuthMiddleware(transactionService.jwt)

	transactionGroup := apiV1.AddGroup("/transaction")

	transactionGroup.HandleFunc("GET /id/{id}", authMiddleware.Authenticate(http.HandlerFunc(t.GetTransactionByIdHandler)).ServeHTTP)
	transactionGroup.HandleFunc("DELETE /{id}", authMiddleware.Authenticate(http.HandlerFunc(t.DeleteTransactionByIdHandler)).ServeHTTP)
	transactionGroup.HandleFunc("GET /", authMiddleware.Authenticate(http.HandlerFunc(t.GetManyTransactionHandler)).ServeHTTP)
}
