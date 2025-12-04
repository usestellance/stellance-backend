package user

import (
	"net/http"

	"github.com/The-True-Hooha/stellance-backend/internal/middleware"
	"github.com/The-True-Hooha/stellance-backend/pkg/httpx"
)

func RegisterUserRoutes(apiV1 *httpx.RouteGroup, router *http.ServeMux, userService *UserService) {
	profileHandler := NewUserHandler(userService)
	authMiddleware := middleware.NewAuthMiddleware(userService.jwtService)
	profileGroup := apiV1.AddGroup("/profile")

	profileGroup.HandleFunc("POST /", authMiddleware.Authenticate(http.HandlerFunc(profileHandler.CompleteProfileHandler)).ServeHTTP)
	profileGroup.HandleFunc("GET /{id}", authMiddleware.Authenticate(http.HandlerFunc(profileHandler.GetProfile)).ServeHTTP)
	profileGroup.HandleFunc("GET /me", authMiddleware.Authenticate(http.HandlerFunc(profileHandler.GetMe)).ServeHTTP)
	profileGroup.HandleFunc("PUT /", authMiddleware.Authenticate(http.HandlerFunc(profileHandler.UpdateProfile)).ServeHTTP)
}
