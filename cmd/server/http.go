package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/The-True-Hooha/stellance-backend.git/internal/auth"
	"github.com/The-True-Hooha/stellance-backend.git/internal/invoice"
	"github.com/The-True-Hooha/stellance-backend.git/internal/middleware"
	"github.com/The-True-Hooha/stellance-backend.git/internal/user"
	"github.com/The-True-Hooha/stellance-backend.git/internal/wallet"
	"github.com/The-True-Hooha/stellance-backend.git/pkg/config"
	"github.com/The-True-Hooha/stellance-backend.git/pkg/config/cors_config"
	"github.com/The-True-Hooha/stellance-backend.git/pkg/httpx"
	"github.com/The-True-Hooha/stellance-backend.git/pkg/logger"
)

type Server struct {
	server *http.Server
	router *http.ServeMux
	logger *slog.Logger
}

type ServerHealthResponse struct {
	Uptime     string                  `json:"uptime"`
	Timestamp  string                  `json:"timestamp"`
	Version    string                  `json:"version"`
	Components []HealthComponentStatus `json:"components,omitempty"`
}

type HealthComponentStatus struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Latency string `json:"latency,omitempty"`
	Error   string `json:"error,omitempty"`
}

var (
	health     int32
	startTime  = time.Now()
	apiVersion = "1.0.0-dev"
)

func SetServerConfig() *Server {
	router := http.NewServeMux()
	redis := config.GetAppContainer().Redis
	config := cors_config.GetCorsConfig()
	cm := middleware.CORSMiddleware(config)
	handler := cm(router)
	handler = middleware.ErrorHandlerMiddleware(handler)
	handler = middleware.LoggerMiddleware(handler)
	handler = middleware.RateLimitGuardMiddleware(redis)(handler)
	log := logger.Logger()

	server := &http.Server{
		Addr:         ":4000",
		Handler:      handler,
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 3 * time.Minute,
		IdleTimeout:  160 * time.Second,
	}
	return &Server{
		server: server,
		logger: log,
		router: router,
	}
}

func (server *Server) AddHttpRoutes() {
	atomic.StoreInt32(&health, 1)
	apiV1 := httpx.NewRouteGroup(server.router, "/api/v1")
	apiV1.HandleFunc("GET /health", runHealthCheck)
	apiV1.HandleFunc("GET /key", generateKey)

	authService := auth.NewAuthService()
	auth.RegisterAuthRoutes(apiV1, server.router, authService)
	profileService := user.NewUserService()
	user.RegisterUserRoutes(apiV1, server.router, profileService)

	invoiceService := invoice.NewInvoiceService()
	invoice.RegisterInvoiceRoutes(apiV1, server.router, invoiceService)

	walletService := wallet.NewWalletService()
	wallet.RegisterWalletRoutes(apiV1, server.router, walletService)
}

func (server *Server) StartHttpServer(ctx context.Context) {
	done := make(chan bool)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	server.AddHttpRoutes()

	go func() {
		select {
		case <-quit:
			server.logger.Info("::: Sever is preparing to shutdown...:::")
			shutdownCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
			defer cancel()

			server.server.SetKeepAlivesEnabled(false)
			if err := server.server.Shutdown(shutdownCtx); err != nil {
				server.logger.Debug("failed to gracefully shutdown the server ", "error", fmt.Sprintf("%+v", err))
			}
			close(done)
		case <-ctx.Done():
			server.logger.Info(":: Server shutdown initiated from context cancellation::")
			server.server.Close()
			close(done)
		}
	}()
	server.logger.Info("Stellance has started and is currently on http://localhost"+server.server.Addr, "event", "sever started")
	if err := server.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		server.logger.Error(":::failed to listen on http://localhost"+server.server.Addr, "event", "sever crashed", "error", err.Error())
	}
	<-done
	server.logger.Info("Server stopped running on http://localhost"+server.server.Addr,
		"event", "shutdown",
		"address", server.server.Addr,
	)
}

func runHealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	statusCode, healthResponse := getHealthStatus()
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(healthResponse)
}

func generateKey(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		return
	}
	w.WriteHeader(http.StatusOK)
	data := map[string]interface{}{
		"key":    key,
		"base64": base64.URLEncoding.EncodeToString(key),
	}
	json.NewEncoder(w).Encode(data)
}

func getHealthStatus() (int, ServerHealthResponse) {
	uptime := time.Since(startTime).Round(time.Second).String()
	response := ServerHealthResponse{
		Uptime:    uptime,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Version:   apiVersion,
	}

	components := []HealthComponentStatus{}
	overallHealthy := true

	pgStatus := checkPostgresStatus()
	components = append(components, pgStatus)
	if pgStatus.Status != "healthy" {
		overallHealthy = false
	}
	redisStatus := checkRedis()
	components = append(components, redisStatus)
	if redisStatus.Status != "healthy" {
		overallHealthy = true
	}

	response.Components = components

	if overallHealthy && atomic.LoadInt32(&health) == 1 {
		return http.StatusOK, response
	}
	return http.StatusServiceUnavailable, response

}

func checkPostgresStatus() HealthComponentStatus {
	start := time.Now()
	status := HealthComponentStatus{
		Name:   "postgres",
		Status: "healthy",
	}

	container := config.GetAppContainer()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := container.Postgres.Ping(ctx); err != nil {
		status.Status = "unhealthy"
		status.Error = err.Error()
	}

	status.Latency = fmt.Sprintf("%.2fms", time.Since(start).Seconds()*1000)
	return status
}

func checkRedis() HealthComponentStatus {
	start := time.Now()
	status := HealthComponentStatus{
		Name:   "redis",
		Status: "healthy",
	}

	container := config.GetAppContainer()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := container.Redis.Ping(ctx).Err(); err != nil {
		status.Status = "unhealthy"
		status.Error = err.Error()
	}

	status.Latency = fmt.Sprintf("%.2fms", time.Since(start).Seconds()*1000)
	return status
}
