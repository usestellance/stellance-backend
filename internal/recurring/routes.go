package recurring

import (
	"net/http"

	"github.com/The-True-Hooha/stellance-backend/internal/middleware"
	jwt_ "github.com/The-True-Hooha/stellance-backend/pkg/jwt"
	"github.com/The-True-Hooha/stellance-backend/pkg/httpx"
)

func RegisterRecurringRoutes(apiV1 *httpx.RouteGroup, router *http.ServeMux, service *RecurringService) {
	h := NewRecurringHandler(service)
	authMiddleware := middleware.NewAuthMiddleware(jwt_.JwtTokenService())
	group := apiV1.AddGroup("/recurring")

	group.HandleFunc("POST /", authMiddleware.Authenticate(http.HandlerFunc(h.Create)).ServeHTTP)
	group.HandleFunc("GET /", authMiddleware.Authenticate(http.HandlerFunc(h.List)).ServeHTTP)
	group.HandleFunc("GET /{id}", authMiddleware.Authenticate(http.HandlerFunc(h.Get)).ServeHTTP)
	group.HandleFunc("PATCH /{id}", authMiddleware.Authenticate(http.HandlerFunc(h.Update)).ServeHTTP)
	group.HandleFunc("DELETE /{id}", authMiddleware.Authenticate(http.HandlerFunc(h.Delete)).ServeHTTP)
}
