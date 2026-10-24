package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/The-True-Hooha/stellance-backend/pkg/config"
	jwt_ "github.com/The-True-Hooha/stellance-backend/pkg/jwt"
	"github.com/The-True-Hooha/stellance-backend/pkg/logger"
	"github.com/jackc/pgx/v5"
)

type AuthMiddleware struct {
	jwt *jwt_.JwtTokenServiceConfig
}

func NewAuthMiddleware(jwt *jwt_.JwtTokenServiceConfig) *AuthMiddleware {
	return &AuthMiddleware{
		jwt: jwt,
	}
}

func (authM *AuthMiddleware) Authenticate(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := logger.Logger()

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			log.Debug("missing authorization header")
			http.Error(w, "please kindly login to complete this request", http.StatusUnauthorized)
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			log.Debug("invalid authorization header format")
			http.Error(w, "invalid request", http.StatusUnauthorized)
			return
		}

		token := parts[1]

		claims, err := jwt_.JwtTokenService().ValidateAccessToken(token)
		if err != nil {
			log.Debug("token validation failed", "error", err)
			http.Error(w, "kindly login to complete this request", http.StatusUnauthorized)
			return
		}

		active, err := userIsActive(r.Context(), claims.UserId)
		if err != nil {
			log.Error("failed to check user status", "error", err)
			http.Error(w, "service unavailable", http.StatusServiceUnavailable)
			return
		}
		if !active {
			http.Error(w, "kindly login to complete this request", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), UserIDKey, claims.UserId)
		ctx = context.WithValue(ctx, UserEmailKey, claims.Email)
		ctx = context.WithValue(ctx, RoleKey, claims.Role)

		log = log.With("user_id", claims.UserId)
		ctx = WriteLoggerToContext(ctx, log)

		h.ServeHTTP(w, r.WithContext(ctx))
	})
}

// userIsActive revokes the sessions of deleted or deactivated users: tokens are stateless.
func userIsActive(ctx context.Context, userID string) (bool, error) {
	var active bool
	err := config.GetAppContainer().Postgres.QueryRow(ctx, `SELECT is_active FROM users WHERE id = $1`, userID).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return active, err
}

func (authM *AuthMiddleware) RequireAdmin(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role, ok := r.Context().Value(RoleKey).(string)
		if !ok || role != "admin" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func (authM *AuthMiddleware) IsPublicAccess(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := logger.Logger()

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			log.Debug("authorization header not provided")
			h.ServeHTTP(w, r)
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			log.Debug("invalid authorization header format")
			h.ServeHTTP(w, r)
			return
		}

		token := parts[1]

		claims, err := jwt_.JwtTokenService().ValidateAccessToken(token)
		if err != nil {
			log.Debug("token validation failed", "error", err)
			h.ServeHTTP(w, r)
			return
		}
		if active, err := userIsActive(r.Context(), claims.UserId); err != nil || !active {
			h.ServeHTTP(w, r)
			return
		}
		ctx := context.WithValue(r.Context(), UserIDKey, claims.UserId)
		ctx = context.WithValue(ctx, UserEmailKey, claims.Email)
		ctx = context.WithValue(ctx, RoleKey, claims.Role)

		log = log.With("user_id", claims.UserId)
		ctx = WriteLoggerToContext(ctx, log)

		h.ServeHTTP(w, r.WithContext(ctx))
	})
}

