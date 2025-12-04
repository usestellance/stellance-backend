package cors_config

import (
	"os"
	"strings"

	"github.com/The-True-Hooha/stellance-backend/pkg/logger"
)

type CORSConfig struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	ExposedHeaders   []string
	AllowCredentials bool
	MaxAge           int
}

var (
	DEFAULT_METHODS = []string{
		"GET",
		"POST",
		"PUT",
		"DELETE",
		"OPTIONS",
		"PATCH",
	}
	ALLOWED_HEADERS = []string{
		"Accept",
		"Authorization",
		"Content-Type",
		"X-CSRF-Token",
		"X-Requested-With",
		"X-Correlation-Id",
		"X-Request-Id",
	}
	EXPOSED_HEADERS = []string{
		"X-Request-Id",
		"X-RateLimit-Limit",
		"X-RateLimit-Remaining",
		"X-RateLimit-Reset",
	}
)

func GetCorsConfig() CORSConfig {
	log := logger.Logger()
	allowedOriginsEnv := os.Getenv("CORS_ALLOWED_ORIGINS")

	var allowedOrigins []string
	if allowedOriginsEnv == "" {
		log.Error("CORS configuration missing: ALLOWED_ORIGINS not set")
		panic("CORS origins unavailable - check environment configuration")
	}

	allowedOrigins = strings.Split(allowedOriginsEnv, ",")
	return CORSConfig{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   DEFAULT_METHODS,
		AllowedHeaders:   ALLOWED_HEADERS,
		ExposedHeaders:   EXPOSED_HEADERS,
		AllowCredentials: true,
		MaxAge:           86400,
	}
}
