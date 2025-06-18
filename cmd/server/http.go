package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/The-True-Hooha/stellance-backend.git/internal/middleware"
	"github.com/The-True-Hooha/stellance-backend.git/pkg/config"
	"github.com/The-True-Hooha/stellance-backend.git/pkg/httpx"
	"github.com/The-True-Hooha/stellance-backend.git/pkg/logger"
)

type Server struct {
	server *http.Server
	router *http.ServeMux
	logger *slog.Logger
	cache  interface{} // TODO: user redis client here
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

var health int32
var startTime = time.Now()
var apiVersion = "1.0.0-dev"

func SetServerConfig() *Server {
	router := http.NewServeMux()
	handler := middleware.ErrorHandlerMiddleware(router)
	handler = middleware.LoggerMiddleware(handler)
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
		cache:  nil,
		router: router,
	}
}

func (server *Server) AddHttpRoutes() {
	atomic.StoreInt32(&health, 1)
	apiV1 := httpx.AddNewRouteGroup("/api/v1")
	apiV1.HandleFunc("GET /health", runHealthCheck)

	usersGroup := apiV1.AddGroup("/users")
	// usersGroup.HandleFunc("GET /", server.handleUserList)
	// usersGroup.HandleFunc("POST /", server.handleUserCreate)

	invoicesGroup := apiV1.AddGroup("/invoices")
	// invoicesGroup.HandleFunc("GET /", server.handleInvoiceList)
	// invoicesGroup.HandleFunc("POST /", server.handleInvoiceCreate)

	apiV1.Inject(server.router)
	usersGroup.Inject(server.router)
	invoicesGroup.Inject(server.router)
	// server.router.HandleFunc("GET /health", runHealthCheck)
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

	if err := container.Database.Ping(ctx); err != nil {
		status.Status = "unhealthy"
		status.Error = err.Error()
	}

	status.Latency = fmt.Sprintf("%.2fms", time.Since(start).Seconds()*1000)
	return status
}

func checkRedis() HealthComponentStatus {
	return HealthComponentStatus{
		Name:   "redis",
		Status: "not_configured",
		Error:  "Redis connection not yet configured",
	}
	// start := time.Now()
	// status := HealthComponentStatus{
	//     Name:   "redis",
	//     Status: "healthy",
	// }
	//
	// ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	// defer cancel()
	//
	// if err := redisClient.Ping(ctx).Err(); err != nil {
	//     status.Status = "unhealthy"
	//     status.Error = err.Error()
	// }
	//
	// status.Latency = time.Since(start).String()
	// return status
}
