package invoice_comments

import (
	"net/http"

	"github.com/The-True-Hooha/stellance-backend/internal/middleware"
	"github.com/The-True-Hooha/stellance-backend/pkg/httpx"
)

func RegisterInvoiceCommentRoutes(apiV1 *httpx.RouteGroup, router *http.ServeMux, service *InvoiceCommentsService) {
	ch := NewInvoiceCommentHandler(service)
	authMiddleware := middleware.NewAuthMiddleware(service.jwtService)
	commentGroup := apiV1.AddGroup("/comments")

	commentGroup.HandleFunc("POST /", authMiddleware.IsPublicAccess(http.HandlerFunc(ch.CreateComment)).ServeHTTP)
	commentGroup.HandleFunc("GET /", authMiddleware.IsPublicAccess(http.HandlerFunc(ch.GetComments)).ServeHTTP)
	commentGroup.HandleFunc("GET /{id}", authMiddleware.IsPublicAccess(http.HandlerFunc(ch.GetCommentByID)).ServeHTTP)
	commentGroup.HandleFunc("PATCH /{id}", authMiddleware.IsPublicAccess(http.HandlerFunc(ch.UpdateComment)).ServeHTTP)
}
