package auth

import (
	"net/http"

	"github.com/The-True-Hooha/stellance-backend/internal/middleware"
	"github.com/The-True-Hooha/stellance-backend/pkg/httpx"
)

func RegisterAuthRoutes(apiV1 *httpx.RouteGroup, router *http.ServeMux, authService *AuthServiceConfig) {
	authHandler := NewAuthHandler(authService)
	authMiddleware := middleware.NewAuthMiddleware(authService.jwt)

	authGroup := apiV1.AddGroup("/auth")
	authGroup.HandleFunc("POST /signup", authHandler.SignUpHandler)
	authGroup.HandleFunc("POST /social", authHandler.SocialSignUpHandler)
	authGroup.HandleFunc("POST /clear", authMiddleware.Authenticate(authMiddleware.RequireAdmin(http.HandlerFunc(authHandler.ClearRedisHandler))).ServeHTTP)
	authGroup.HandleFunc("GET /resend-email", authHandler.ResendEmailVerification)
	authGroup.HandleFunc("POST /login", authHandler.LoginHandler)
	authGroup.HandleFunc("GET /validate", authHandler.ValidateEmailHandler)
	authGroup.HandleFunc("GET /reset", authHandler.RequestPasswordReset)
	authGroup.HandleFunc("POST /reset-password", authHandler.UpdatePassword)
	authGroup.HandleFunc("POST /change-password", authMiddleware.Authenticate(http.HandlerFunc(authHandler.ChangePassword)).ServeHTTP)
	authGroup.HandleFunc("POST /token", authMiddleware.Authenticate(http.HandlerFunc(authHandler.RefreshTokenHandler)).ServeHTTP)
}
