package notifications

import (
	"net/http"

	"github.com/The-True-Hooha/stellance-backend/internal/middleware"
	"github.com/The-True-Hooha/stellance-backend/pkg/httpx"
)

func RegisterNotificationRoutes(apiV1 *httpx.RouteGroup, router *http.ServeMux, ns *NotificationService) {
	nh := NewNotificationHandler(ns)

	authMiddleware := middleware.NewAuthMiddleware(ns.jwt)

	ng := apiV1.AddGroup("/notification")

	ng.HandleFunc("GET /{id}", authMiddleware.Authenticate(http.HandlerFunc(nh.GetNotificationById)).ServeHTTP)
	ng.HandleFunc("DELETE /{id}", authMiddleware.Authenticate(http.HandlerFunc(nh.DeleteNotificationById)).ServeHTTP)
	ng.HandleFunc("GET /", authMiddleware.Authenticate(http.HandlerFunc(nh.GetNotifications)).ServeHTTP)
	ng.HandleFunc("PATCH /{id}", authMiddleware.Authenticate(http.HandlerFunc(nh.EditNotificationViewStatus)).ServeHTTP)

}
