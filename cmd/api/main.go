// @title Stellance API
// @version 1.0
// @description Invoice management and Stellar payment platform API.
// @description
// @description **Envelope.** Every JSON response is `{status_code, message, data?, error?}`; the types below describe `data`.
// @description **Auth.** Send `Authorization: Bearer <access_token>`. Tokens last 1 hour; deleted or deactivated users are rejected immediately. Unverified emails cannot sign in.
// @description **Pagination.** List endpoints take `page` (from 1) and `limit`, and return `{items, meta:{page, limit, total, total_pages}}`. Old names (`page_count`, `count`, `payments`, `invoice`, `data`, `notifications`) still work and are deprecated.
// @description **Amounts.** Payment records and requests use 7-decimal strings (`"2.0300000"`). Invoice and ledger fields are JSON numbers in the invoice currency. `/transaction/stats` and `/transaction/inflow` are integer cents. See docs/api-conventions.md.
// @description **Payments (v2).** Invoices are paid with a text memo equal to the invoice's `payment_reference`. The server detects and verifies the on-chain payment; only a verified payment marks an invoice paid. Status values and transitions are in docs/payment-status-model.md.
// @description **Errors.** 400 invalid input, 401 missing or invalid token, 403 not allowed, 404 not found, 409 state conflict, 422 valid request that fails a business rule, 502 Stellar unreachable, 500 unexpected.
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.email davidhero125@gmail.com

// @license.name MIT

// @tag.name auth
// @tag.description Sign-up, sign-in (password and Google), email verification and password reset
// @tag.name profile
// @tag.description Profile of the signed-in user and its primary wallet
// @tag.name invoices
// @tag.description Create, send, review, search and download invoices
// @tag.name payments-v2
// @tag.description Payment requests, detected on-chain payments and their verification results
// @tag.name wallet
// @tag.description Stellar wallet, PIN, transfers and invoice payments
// @tag.name transactions
// @tag.description Ledger of money movements and cash-flow statistics
// @tag.name notifications
// @tag.description In-app notifications, including payment outcomes
// @tag.name comments
// @tag.description Invoice comment threads and reactions
// @tag.name recurring
// @tag.description Recurring invoice schedules
// @tag.name admin
// @tag.description Admin-only management of users, invoices, transactions, payments and network settings

// @host localhost:4000
// @BasePath /api/v1

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Enter "Bearer {token}"

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	_ "github.com/The-True-Hooha/stellance-backend/docs"
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
	mux.HandleFunc(tasks.TypeGenerateRecurringInvoices, tasks.HandleGenerateRecurringInvoices)
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

	recurringTask, err := tasks.NewGenerateRecurringInvoicesTask()
	if err != nil {
		log.Error("failed to create recurring invoice task", "error", err)
	} else {
		if _, err := scheduler.Register("0 1 * * *", recurringTask); err != nil {
			log.Error("failed to register recurring invoice task", "error", err)
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
