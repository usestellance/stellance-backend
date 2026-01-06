package main

import (
	"context"
	"fmt"
	"os"

	"github.com/The-True-Hooha/stellance-backend/cmd/server"
	database "github.com/The-True-Hooha/stellance-backend/internal/db"
	"github.com/The-True-Hooha/stellance-backend/internal/middleware"
	"github.com/The-True-Hooha/stellance-backend/internal/tasks"
	"github.com/The-True-Hooha/stellance-backend/pkg/config"
	"github.com/The-True-Hooha/stellance-backend/pkg/logger"
	"github.com/The-True-Hooha/stellance-backend/pkg/utils"
	"github.com/hibiken/asynq"
	"github.com/joho/godotenv"
)

func main() {
	log := logger.Logger()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = middleware.WriteLoggerToContext(ctx, log)
	stage := os.Getenv("STAGE")

	if stage != "prod" {
		err := godotenv.Load(".env")
		if err != nil {
			log.Error("Error loading the .env file:", "error", err)
		}
	}

	pg := database.PostgresConfig{
		Name:     os.Getenv("PG_NAME"),
		Port:     utils.GetEnvAsInt(),
		Host:     os.Getenv("PG_HOST"),
		User:     os.Getenv("PG_USER"),
		Password: os.Getenv("PG_PASSWORD"),
		Stage:    os.Getenv("STAGE"),
	}

	redis := database.RedisConfig{
		Host:     os.Getenv("REDIS_HOST"),
		Port:     os.Getenv("REDIS_PORT"),
		Password: os.Getenv("REDIS_PASSWORD"),
		Index:    0,
		Stage:    os.Getenv("STAGE"),
	}

	err := config.InitializeContainer(ctx, pg, log, redis)
	if err != nil {
		log.Error("failed to initialize app container", "error", err.Error())
		os.Exit(1)
	}

	defer config.Shutdown()

	server := server.SetServerConfig()
	server.StartHttpServer(ctx)
	addr := fmt.Sprintf("%s:%s", redis.Host, redis.Port)

	scheduler := asynq.NewScheduler(asynq.RedisClientOpt{
		Addr:     addr,
		Password: redis.Password,
		DB:       redis.Index,
	}, nil)
	task, err := tasks.NewUpdateOverdueInvoicesTask()
	if err != nil {
		log.Error("failed to run scheduler to update invoices tasks")
	} else {
		scheduler.Register("0 1 * * *", task)
		err := scheduler.Run()
		if err != nil {
			log.Error("failed to start scheduler", "error", err)
		}
	}

}
