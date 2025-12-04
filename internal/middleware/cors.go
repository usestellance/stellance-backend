package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/The-True-Hooha/stellance-backend/pkg/config/cors_config"
	"github.com/The-True-Hooha/stellance-backend/pkg/logger"
)

func CORSMiddleware(config cors_config.CORSConfig) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			if isOriginAllowed(origin, config.AllowedOrigins) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", strings.Join(config.AllowedMethods, ", "))
				w.Header().Set("Access-Control-Allow-Headers", strings.Join(config.AllowedHeaders, ", "))

				if len(config.ExposedHeaders) > 0 {
					w.Header().Set("Access-Control-Expose-Headers", strings.Join(config.ExposedHeaders, ", "))
				}

				if config.MaxAge > 0 {
					w.Header().Set("Access-Control-Max-Age", fmt.Sprintf("%d", config.MaxAge))
				}
			}

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			h.ServeHTTP(w, r)

		})
	}
}

func isOriginAllowed(origin string, allowedOrigins []string) bool {
	log := logger.Logger()

	origin = strings.TrimSpace(origin)
	if origin == "" {
		log.Warn("CORS: origin is empty")
		return false
	}

	for _, allowed := range allowedOrigins {
		allowed = strings.TrimSpace(allowed)

		switch {
		case allowed == "*":
			return true

		case strings.HasPrefix(allowed, "*"):
			domain := strings.TrimPrefix(allowed, "*")
			if strings.HasSuffix(origin, domain) {
				return true
			}

		case origin == allowed:
			return true

		case strings.Contains(allowed, "localhost") && strings.Contains(origin, "localhost"):
			return true
		}
	}

	log.Warn("CORS: origin not allowed", "origin", origin)
	return false
}
