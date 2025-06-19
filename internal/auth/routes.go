package auth

import (
	"net/http"

	"github.com/The-True-Hooha/stellance-backend.git/pkg/httpx"
)

func RegisterAuthRoutes(apiV1 *httpx.RouteGroup, router *http.ServeMux, authService *AuthServiceConfig) {
	authHandler := NewAuthHandler(authService)

	authGroup := apiV1.AddGroup("/auth")
	authGroup.HandleFunc("POST /signup", authHandler.SignUpHandler)
	authGroup.HandleFunc("POST /login", authHandler.LoginHandler)
	authGroup.HandleFunc("GET /refresh-token", authHandler.RefreshTokenHandler)
	authGroup.HandleFunc("GET /validate", authHandler.ValidateEmailHandler)

	authGroup.Inject(router)
}
