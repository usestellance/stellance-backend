package admin

import (
	"net/http"

	"github.com/The-True-Hooha/stellance-backend/internal/middleware"
	jwt_ "github.com/The-True-Hooha/stellance-backend/pkg/jwt"
	"github.com/The-True-Hooha/stellance-backend/pkg/httpx"
)

func RegisterAdminRoutes(apiV1 *httpx.RouteGroup, router *http.ServeMux, service *AdminService) {
	h := NewAdminHandler(service)
	authMiddleware := middleware.NewAuthMiddleware(jwt_.JwtTokenService())
	group := apiV1.AddGroup("/admin")

	admin := func(handler http.HandlerFunc) http.HandlerFunc {
		return authMiddleware.Authenticate(authMiddleware.RequireAdmin(handler)).ServeHTTP
	}

	group.HandleFunc("GET /stats", admin(h.GetStats))
	group.HandleFunc("GET /users", admin(h.ListUsers))
	group.HandleFunc("GET /users/{id}", admin(h.GetUser))
	group.HandleFunc("PATCH /users/{id}/activate", admin(h.ActivateUser))
	group.HandleFunc("PATCH /users/{id}/deactivate", admin(h.DeactivateUser))
	group.HandleFunc("DELETE /users/{id}", admin(h.DeleteUser))
	group.HandleFunc("GET /invoices", admin(h.ListInvoices))
	group.HandleFunc("GET /invoices/{id}", admin(h.GetInvoice))
	group.HandleFunc("GET /transactions", admin(h.ListTransactions))
}
