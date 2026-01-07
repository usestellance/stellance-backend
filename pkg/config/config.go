package config

import (
	"context"
	"log/slog"
	"sync"

	database "github.com/The-True-Hooha/stellance-backend/internal/db"
	"github.com/The-True-Hooha/stellance-backend/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type AppContainer struct {
	Postgres *pgxpool.Pool
	Log      *slog.Logger
	Redis    *redis.Client
	Storage  *storage.S3Storage
}

var (
	container *AppContainer
	once      sync.Once
)

func InitializeContainer(ctx context.Context, dbConfig database.PostgresConfig, log *slog.Logger, redisConfig database.RedisConfig, storageConfig storage.S3Config) error {
	var initError error

	once.Do(func() {
		pool, err := database.CreateNewPostgresConnection(ctx, dbConfig)
		if err != nil {
			log.Error("failed to initialize postgres pool", "error", err.Error())
			initError = err
			return
		}
		redisClient, err := database.CreateNewRedisClient(redisConfig)
		if err != nil {
			log.Error("failed to initialize Redis client", "error", err.Error())
			initError = err
			return
		}

		storage, err := storage.NewS3Storage(storageConfig)
		if err != nil {
			log.Error("failed to initialize railway storage client", "error", err)
			initError = err
			return
		}

		log.Info("All services initialized successfully")
		container = &AppContainer{
			Postgres: pool.Pool,
			Redis:    redisClient,
			Log:      log,
			Storage:  storage,
		}
	})

	return initError
}

func GetAppContainer() *AppContainer {
	if container == nil {
		panic("App container not initialized. Call Initialize container first")
	}
	return container
}

func Shutdown() {
	if container != nil {
		if container.Postgres != nil {
			container.Log.Info("closing Postgres connection pool")
			container.Postgres.Close()
		}
		if container.Redis != nil {
			container.Log.Info("closing Redis connection")
			container.Redis.Close()
		}
	}
}
