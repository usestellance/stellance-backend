package invoice_comments

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/The-True-Hooha/stellance-backend/pkg/config"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type InvoiceCommentsService struct {
	log      *slog.Logger
	postgres *pgxpool.Pool
	redis    *redis.Client
}

func NewInvoiceCommentService() *InvoiceCommentsService {
	return &InvoiceCommentsService{
		log:      config.GetAppContainer().Log,
		postgres: config.GetAppContainer().Postgres,
		redis:    config.GetAppContainer().Redis,
	}
}

func (ins *InvoiceCommentsService) CreateCommentQuery(ctx context.Context, comment CreateCommentDTO) (*CreateCommentQueryResponse, error) {
	email := strings.ToLower(comment.CommenterEmail)

	tx, err := ins.postgres.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.ReadCommitted,
		AccessMode: pgx.ReadWrite,
	})

	if err != nil {
		ins.log.Error("failed to start database transaction", "error", err)
		return nil, fmt.Errorf("failed to started process, internal error: %w", err)
	}
	defer tx.Rollback(ctx)

	var (
		ID         string
		Created_at time.Time
		Verified   bool
		Guest      bool
	)

	const query = `
	INSERT INTO invoice_comments (
		invoice_id, user_id, commenter_name, commenter_email, 
		comment_text, parent_comment_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, is_verified, is_guest
		`
	err = tx.QueryRow(ctx, query, comment.InvoiceID, comment.UserID, comment.CommenterName, email,
		comment.CommentText, comment.ParentID).Scan(&ID, &Created_at, &Verified, &Guest)
	if err != nil {
		ins.log.Error("failed to execute query to start insert operation on invoice comments", "error", err)
		return nil, fmt.Errorf("failed to create invoice: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		ins.log.Error("failed to commit transaction on creating new invoice comment", "error", err)
		return nil, fmt.Errorf("failed to create invoice comment: %w", err)
	}

	ins.log.Info("new invoice comment added", "comment", comment)
	return &CreateCommentQueryResponse{
		ID:         ID,
		Created_at: Created_at,
		Verified:   Verified,
		Guest:      Guest,
	}, nil
}

func (ic *InvoiceCommentsService) GetCommentByIDQuery(ctx context.Context, commentID string) (*InvoiceComment, error) {
	const query = `
		SELECT id, invoice_id, user_id, commenter_name, commenter_email,
		comment_text, is_verified, is_guest, parent_comment_id, edited,
		edited_at, created_at, updated_at
		FROM invoice_comments WHERE id = $1
	`

	var comment InvoiceComment

	err := ic.postgres.QueryRow(ctx, query, commentID).Scan(
		&comment.ID,
		&comment.InvoiceID,
		&comment.UserID,
		&comment.CommenterName,
		&comment.CommenterEmail,
		&comment.CommentText,
		&comment.IsVerified,
		&comment.IsGuest,
		&comment.ParentCommentID,
		&comment.Edited,
		&comment.EditedAt,
		&comment.CreatedAt,
		&comment.UpdatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			ic.log.Error("queried failed: comment ID does not exist", "comment_id", commentID, "error", err)
			return nil, fmt.Errorf("comment not found with id: %s, %w", commentID, err)
		}
		ic.log.Error("error occurred fetching invoice comment by id", "error", err, "comment_id", commentID)
		return nil, fmt.Errorf("failed to get comment: %w", err)
	}

	return &comment, nil
}
