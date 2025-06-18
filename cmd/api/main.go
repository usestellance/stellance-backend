package main

import (
	"context"
	"os"

	"github.com/The-True-Hooha/stellance-backend.git/cmd/server"
	database "github.com/The-True-Hooha/stellance-backend.git/internal/db"
	"github.com/The-True-Hooha/stellance-backend.git/internal/middleware"
	"github.com/The-True-Hooha/stellance-backend.git/pkg/config"
	"github.com/The-True-Hooha/stellance-backend.git/pkg/logger"
	"github.com/The-True-Hooha/stellance-backend.git/pkg/utils"
	"github.com/joho/godotenv"
)

func main() {
	log := logger.Logger()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctx = middleware.WriteLoggerToContext(ctx, log)

	if os.Getenv("STAGE") != "prod" {
		err := godotenv.Load(".env")
		if err != nil {
			log.Error("Error loading the .env file:", "error", err)
		}
	}

	pg := database.PostgresConfig{
		Name:     os.Getenv("PG_NAME"),
		Port:     utils.GetEnvAsInt(),
		Host:     os.Getenv("PB_HOST"),
		User:     os.Getenv("PG_USER"),
		Password: os.Getenv("PG_PASSWORD"),
		Stage:    os.Getenv("STAGE"),
	}

	err := config.InitializeContainer(ctx, pg, log)
	if err != nil {
		log.Error("failed to initialize app container", "error", err.Error())
		os.Exit(1)
	}

	defer config.Shutdown()

	server := server.SetServerConfig()
	server.StartHttpServer(ctx)
}
