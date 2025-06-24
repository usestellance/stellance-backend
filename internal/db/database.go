package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresConfig struct {
	Name     string
	Port     int
	Host     string
	User     string
	Password string
	Stage    string
}

type PgPool struct {
	*pgxpool.Pool
}

func CreateNewPostgresConnection(ctx context.Context, config PostgresConfig) (*PgPool, error) {
	var sslMode string
	if config.Stage == "dev" {
		sslMode = "disable"
	} else {
		sslMode = "require"
	}

	url := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		config.User, config.Password, config.Host, config.Port, config.Name, sslMode)
	pg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	pg.MaxConns = 25
	pg.MinConns = 5
	pg.MaxConnLifetime = 1 * time.Hour
	pg.ConnConfig.ConnectTimeout = 5 * time.Second
	pg.MaxConnIdleTime = 15 * time.Minute
	pg.HealthCheckPeriod = 3 * time.Minute
	pg.ConnConfig.RuntimeParams = map[string]string{
		"statement_timeout":                   "50000",
		"idle_in_transaction_session_timeout": "50000",
	}

	pg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		return nil
	}
	maxRetries := 3
	retryDelay := 2 * time.Second
	for i := 0; i < maxRetries; i++ {
		pool, err := pgxpool.NewWithConfig(ctx, pg)
		if err == nil {
			if err := pool.Ping(ctx); err == nil {
				return &PgPool{
					Pool: pool,
				}, nil
			}
			pool.Close()
		}
		if i < maxRetries-1 {
			time.Sleep(retryDelay)
			retryDelay *= 5
		}
	}
	return nil, fmt.Errorf("failed to connect after %d attempts: %w", maxRetries, err)

}
