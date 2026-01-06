package transactions

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/The-True-Hooha/stellance-backend/internal/notifications"
	"github.com/The-True-Hooha/stellance-backend/pkg/config"
	jwt_ "github.com/The-True-Hooha/stellance-backend/pkg/jwt"
	"github.com/The-True-Hooha/stellance-backend/pkg/utils"
	"github.com/jackc/pgx"
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
		currency,
		dto.TransactionStatus,
		dto.NetworkFee,
		currency,
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

	go func() {
		body := fmt.Sprintf("A new transaction has occurred in your wallet. %.2f %s has been added to your %s wallet.", dto.Amount, currency, currency)
		data := notifications.CreateNotificationDto{
			Title:  "New Transaction Update",
			UserId: userId,
			Body:   body,
		}
		notifications.NewNotificationService().CreateNewNotification(context.Background(), data)
	}()

	return true, nil
}

func (s *TransactionService) GetTransactionByID(ctx context.Context, id, user_id string) *utils.ApiResponse {
	const query = `
		SELECT id, invoice_id, wallet_id, transaction_hash, amount, currency,
			   status, network_fee, created_at, confirmed_at, token_type, transaction_type
		FROM transactions WHERE id = $1 AND user_id = $2
	`

	var t GetTransactionDto
	err := s.postgres.QueryRow(ctx, query, id, user_id).Scan(
		&t.ID,
		&t.InvoiceID,
		&t.WalletID,
		&t.TransactionHash,
		&t.Amount,
		&t.Currency,
		&t.Status,
		&t.NetworkFee,
		&t.CreatedAt,
		&t.ConfirmedAt,
		&t.TokenType,
		&t.TransactionType,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return &utils.ApiResponse{
				StatusCode: http.StatusNotFound,
				Message:    fmt.Sprintf("transaction with id '%s' does not exist", id),
			}
		}
		s.log.Error("failed to fetch user", "error", err, "user_id", user_id)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to process request. Please try again.",
		}
	}

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "successful",
		Data:       &t,
	}
}

func (s *TransactionService) DeleteTransactionByID(ctx context.Context, id, user_d string) *utils.ApiResponse {
	const query = `DELETE FROM transactions WHERE id = $1 AND user_id = $2`

	cmdTag, err := s.postgres.Exec(ctx, query, id, user_d)
	if err != nil {
		s.log.Error("failed to delete transaction", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "sever is currently available",
		}
	}
	if cmdTag.RowsAffected() == 0 {
		s.log.Warn("no transaction found to delete", "id", id)
		return &utils.ApiResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("transaction with id '%s' does not exist", id),
		}
	}
	s.log.Info("transaction deleted successfully", "id", id)
	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "transaction deleted successfully",
	}
}

func (s *TransactionService) GetTransactionsPaginated(ctx context.Context, page, limit int, user_id string) *utils.ApiResponse {
	offset := (page - 1) * limit

	const query = `
		SELECT id, invoice_id, wallet_id, transaction_hash, amount, currency,
	   	status, network_fee, created_at, confirmed_at, token_type, transaction_type
		FROM transactions
		WHERE user_id = $3
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := s.postgres.Query(ctx, query, limit, offset, user_id)
	if err != nil {
		s.log.Error("failed to fetch transactions", "error", err)
		if err == pgx.ErrNoRows {
			return &utils.ApiResponse{
				StatusCode: http.StatusNotFound,
				Message:    "User does not seem to have any transaction",
			}
		}
		s.log.Error("failed to fetch user", "error", err, "user_id", user_id)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to process request. Please try again.",
		}
	}
	defer rows.Close()

	var transactions []GetTransactionDto
	for rows.Next() {
		var t GetTransactionDto
		if err := rows.Scan(
			&t.ID,
			&t.InvoiceID,
			&t.WalletID,
			&t.TransactionHash,
			&t.Amount,
			&t.Currency,
			&t.Status,
			&t.NetworkFee,
			&t.CreatedAt,
			&t.ConfirmedAt,
			&t.TokenType,
			&t.TransactionType,
		); err != nil {
			s.log.Error("failed to scan transaction", "error", err)
			continue
		}
		transactions = append(transactions, t)
	}

	var totalCount int
	err = s.postgres.QueryRow(ctx, `SELECT COUNT(*) FROM transactions`).Scan(&totalCount)
	if err != nil {
		s.log.Error("failed to count transactions", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "sever is currently available",
		}
	}

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "successful",
		Data: &PaginatedTransactions{
			Data:       transactions,
			Page:       page,
			Limit:      limit,
			TotalCount: totalCount,
		},
	}
}

func (tr *TransactionService) GetTransactionOverviewByUserQuery(ctx context.Context, userID string) ([]TransactionOverviewRow, error) {
    const query = `
        SELECT 
            COALESCE(i.status::text, 'no_invoice') as invoice_status,
            COALESCE(SUM(t.amount), 0) as total_amount,
            COUNT(DISTINCT t.invoice_id) as invoice_count
        FROM transactions t
        LEFT JOIN invoice i ON t.invoice_id = i.id
        WHERE t.user_id = $1
            AND t.currency = 'usdc'
            AND t.status = 'confirmed'
        GROUP BY i.status
    `
    
    rows, err := tr.postgres.Query(ctx, query, userID)
    if err != nil {
        return nil, fmt.Errorf("failed to query transaction overview: %w", err)
    }
    defer rows.Close()
    
    var results []TransactionOverviewRow
    for rows.Next() {
        var row TransactionOverviewRow
        if err := rows.Scan(&row.InvoiceStatus, &row.TotalAmount, &row.InvoiceCount); err != nil {
            return nil, fmt.Errorf("failed to scan row: %w", err)
        }
        results = append(results, row)
    }
    
    if err := rows.Err(); err != nil {
        return nil, fmt.Errorf("row iteration error: %w", err)
    }
    
    return results, nil
}

func (ts *TransactionService) GetTransactionCardForUser(ctx context.Context, userID string) *utils.ApiResponse {
    overviewData, err := ts.GetTransactionOverviewByUserQuery(ctx, userID)
    if err != nil {
        return &utils.ApiResponse{
            StatusCode: http.StatusInternalServerError,
            Message:    "failed to retrieve transaction overview",
            Error:      err.Error(),
        }
    }
    
    response := GetTransactionOverViewResponse{
        TotalAmount:   TransactionCard{Amount: 0, InvoiceCount: 0},
        PendingAmount: TransactionCard{Amount: 0, InvoiceCount: 0},
        PaidAmount:    TransactionCard{Amount: 0, InvoiceCount: 0},
        OverdueAmount: TransactionCard{Amount: 0, InvoiceCount: 0},
    }
    
    for _, row := range overviewData {
        amountInCents, err := utils.ConvertUSDCToCents(row.TotalAmount)
        if err != nil {
			ts.log.Error("failed to parse USDC amount to cents", "error", err)
            continue
        }
        
        response.TotalAmount.Amount += amountInCents
        response.TotalAmount.InvoiceCount += row.InvoiceCount

        switch row.InvoiceStatus {
        case "pending", "draft", "sent", "viewed":
            response.PendingAmount.Amount += amountInCents
            response.PendingAmount.InvoiceCount += row.InvoiceCount
        case "paid":
            response.PaidAmount.Amount += amountInCents
            response.PaidAmount.InvoiceCount += row.InvoiceCount
        case "overdue":
            response.OverdueAmount.Amount += amountInCents
            response.OverdueAmount.InvoiceCount += row.InvoiceCount
        }
    }
    
    return &utils.ApiResponse{
        StatusCode: http.StatusOK,
        Message:    "successful",
        Data:       response,
    }
}
