package transactions

import (
	"context"
	"log/slog"

	"github.com/The-True-Hooha/stellance-backend.git/pkg/config"
	jwt_ "github.com/The-True-Hooha/stellance-backend.git/pkg/jwt"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type TransactionService struct {
	log      *slog.Logger
	postgres *pgxpool.Pool
	redis    *redis.Client
	jwt      *jwt_.JwtTokenServiceConfig
}

func NewTransactionService() *TransactionService {
	return &TransactionService{
		log:      config.GetAppContainer().Log,
		postgres: config.GetAppContainer().Postgres,
		redis:    config.GetAppContainer().Redis,
		jwt:      jwt_.JwtTokenService(),
	}
}

func (ts *TransactionService) CreateNewTransaction(ctx context.Context, userId string, dto TransactionDto, currency string) (bool, error) {
    log := ts.log
    tx, err := ts.postgres.Begin(ctx)
    if err != nil {
        log.Error("failed to start postgres transaction query", "error", err)
        return false, err
    }
    defer tx.Rollback(ctx)

    const transactionQ string = `
        INSERT INTO transactions (
            invoice_id, wallet_id, transaction_hash, amount,
            currency, status, network_fee, token_type, confirmed_at,
            transaction_type, user_id
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`
    
    _, err = tx.Exec(ctx, transactionQ,
        dto.InvoiceID,
        dto.WalletID,
        dto.TransactionHash,
        dto.Amount,
        currency,           // currency
        dto.TransactionStatus,
        dto.NetworkFee,
        currency,           // token_type (same as currency?)
        dto.ConfirmedAt,
        dto.TransactionType,
        userId,
    )

    if err != nil {
        log.Error("failed to insert new transaction values", "error", err)
        return false, err
    }
    
    if err := tx.Commit(ctx); err != nil {
        log.Error("failed to commit new transaction on wallet", "error", err)
        return false, err
    }
    
    return true, nil
}