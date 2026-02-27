package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/The-True-Hooha/stellance-backend/cmd/server"
	database "github.com/The-True-Hooha/stellance-backend/internal/db"
	"github.com/The-True-Hooha/stellance-backend/internal/middleware"
	"github.com/The-True-Hooha/stellance-backend/internal/storage"
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
	s3Storage := storage.S3Config{
		AccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
		BucketName:      os.Getenv("AWS_S3_BUCKET_NAME"),
		Region:          os.Getenv("AWS_DEFAULT_REGION"),
		Endpoint:        "https://storage.railway.app",
	}

	err := config.InitializeContainer(ctx, pg, log, redis, s3Storage)
	if err != nil {
		log.Error("failed to initialize app container", "error", err.Error())
		os.Exit(1)
	}

	defer config.Shutdown()

	addr := fmt.Sprintf("%s:%s", redis.Host, redis.Port)
	redisOpt := asynq.RedisClientOpt{
		Addr:     addr,
		Password: redis.Password,
		DB:       redis.Index,
	}

	worker, scheduler := startBackgroundWorkers(ctx, redisOpt, log)
	defer func() {
		scheduler.Shutdown()
		worker.Shutdown()
	}()

	server := server.SetServerConfig()
	server.StartHttpServer(ctx)
}

func startBackgroundWorkers(ctx context.Context, redisOpt asynq.RedisClientOpt, log *slog.Logger) (*asynq.Server, *asynq.Scheduler) {
	worker := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: 10,
	})

	mux := asynq.NewServeMux()
	mux.HandleFunc(tasks.TypeUpdateOverdueInvoices, tasks.HandleUpdateOverdueInvoices)
	go func() {
		if err := worker.Run(mux); err != nil {
			log.Error("asynq worker stopped running", "error", err)
		}
	}()

	scheduler := asynq.NewScheduler(redisOpt, nil)
	task, err := tasks.NewUpdateOverdueInvoicesTask()
	if err != nil {
		log.Error("failed to create update invoice overdue tasks", "error", err)
	} else {
		if _, err := scheduler.Register("0 0 * * *", task); err != nil {
			log.Error("failed to register scheduled task", "error", err)
		}
	}

	go func() {
		if err := scheduler.Run(); err != nil {
			log.Error("asynq scheduler stopped", "error", err)
		}
	}()
	log.Info("Background workers started",
		"scheduler_cron", "0 1 * * *",
		"worker_concurrency", 10,
	)

	return worker, scheduler
}
