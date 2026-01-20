package invoice_comments

import (
	"context"

	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/The-True-Hooha/stellance-backend/pkg/config"
	jwt_ "github.com/The-True-Hooha/stellance-backend/pkg/jwt"
	"github.com/The-True-Hooha/stellance-backend/pkg/utils"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type InvoiceCommentsService struct {
	log        *slog.Logger
	postgres   *pgxpool.Pool
	redis      *redis.Client
	jwtService *jwt_.JwtTokenServiceConfig
}

func NewInvoiceCommentService() *InvoiceCommentsService {
	return &InvoiceCommentsService{
		log:        config.GetAppContainer().Log,
		postgres:   config.GetAppContainer().Postgres,
		redis:      config.GetAppContainer().Redis,
		jwtService: jwt_.JwtTokenService(),
	}
}

func (ic *InvoiceCommentsService) GetUserByIDQuery(ctx context.Context, userID string) (*GetUserDto, error) {
	const query = `
		SELECT id, first_name, last_name, email FROM users WHERE id = $1 AND is_active = true
	`

	var user GetUserDto
	err := ic.postgres.QueryRow(ctx, query, userID).Scan(&user.UserId, &user.FirstName, &user.LastName, &user.Email)
	if err != nil {
		if err == pgx.ErrNoRows {
			ic.log.Error("queried failed: user ID does not exist", "user_id", userID, "error", err)
			return nil, fmt.Errorf("user not found with id: %s, %w", userID, err)
		}
		ic.log.Error("error occurred fetching user by id", "error", err, "comment_id", userID)
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &user, nil
}

func (ic *InvoiceCommentsService) GetInvoiceByIDQuery(ctx context.Context, invoiceID string) (*GetInvoiceDto, error) {
	const query = `
		SELECT id, created_by_id, invoice_url, invoice_number, status FROM invoice WHERE id = $1
	`
	var res GetInvoiceDto
	err := ic.postgres.QueryRow(ctx, query, invoiceID).Scan(&res.ID, &res.UserID, &res.InvoiceURL, &res.InvoiceNumber, &res.InvoiceStatus)

	if err != nil {
		if err == pgx.ErrNoRows {
			ic.log.Error("queried failed: invoice ID does not exist", "invoice_id", invoiceID, "error", err)
			return nil, fmt.Errorf("invoice not found with id: %s, %w", invoiceID, err)
		}
		ic.log.Error("error occurred fetching invoice by id", "error", err, "invoice_id", invoiceID)
		return nil, fmt.Errorf("failed to get invoice: %w", err)
	}

	return &GetInvoiceDto{
		ID:            res.ID,
		UserID:        res.UserID,
		InvoiceURL:    res.InvoiceURL,
		InvoiceNumber: res.InvoiceNumber,
		InvoiceStatus: res.InvoiceStatus,
	}, nil

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

	
	var userID interface{} = nil
	if comment.UserID != "" {
		userID = comment.UserID
	}

	var parentID interface{} = nil
	if comment.ParentID != "" {
		parentID = comment.ParentID
	}

	const query = `
	INSERT INTO invoice_comments (
		invoice_id, user_id, commenter_name, commenter_email, 
		comment_text, parent_comment_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, is_verified, is_guest
		`
	err = tx.QueryRow(ctx, query, comment.InvoiceID, userID, comment.CommenterName, email,
		comment.CommentText, parentID).Scan(&ID, &Created_at, &Verified, &Guest)
	if err != nil {
		ins.log.Error("failed to execute query to insert invoice comment", "error", err)
		return nil, fmt.Errorf("failed to create invoice comment: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		ins.log.Error("failed to commit transaction on creating new invoice comment", "error", err)
		return nil, fmt.Errorf("failed to create invoice comment: %w", err)
	}

	ins.log.Info("new invoice comment added", "comment_id", ID)
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

func (ic *InvoiceCommentsService) GetCommentByInvoiceID(ctx context.Context, invoiceID string, parentID *string, limit, offset int, sortOrder string) ([]InvoiceComment, int, error) {
	var query string
	var countQuery string
	var args []interface{}

	if sortOrder == "" {
		sortOrder = "ASC"
	}

	if parentID == nil {
		query = fmt.Sprintf(`
			SELECT id, invoice_id, user_id, commenter_name, commenter_email,
			       comment_text, is_verified, is_guest, parent_comment_id,
			       edited, edited_at, created_at, updated_at
			FROM invoice_comments
			WHERE invoice_id = $1 AND parent_comment_id IS NULL
			ORDER BY created_at %s
			LIMIT $2 OFFSET $3
		`, sortOrder)

		countQuery = `
			SELECT COUNT(*)
			FROM invoice_comments
			WHERE invoice_id = $1 AND parent_comment_id IS NULL
		`
		args = []interface{}{invoiceID, limit, offset}
	} else {
		query = fmt.Sprintf(`
			SELECT id, invoice_id, user_id, commenter_name, commenter_email,
			       comment_text, is_verified, is_guest, parent_comment_id,
			       edited, edited_at, created_at, updated_at
			FROM invoice_comments
			WHERE invoice_id = $1 AND parent_comment_id = $2
			ORDER BY created_at %s
			LIMIT $3 OFFSET $4
		`, sortOrder)

		countQuery = `
			SELECT COUNT(*)
			FROM invoice_comments
			WHERE invoice_id = $1 AND parent_comment_id = $2
		`
		args = []interface{}{invoiceID, *parentID, limit, offset}
	}

	var total int
	var countArgs []interface{}
	if parentID == nil {
		countArgs = []interface{}{invoiceID}
	} else {
		countArgs = []interface{}{invoiceID, *parentID}
	}

	err := ic.postgres.QueryRow(ctx, countQuery, countArgs...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count comments: %w", err)
	}

	rows, err := ic.postgres.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query comments: %w", err)
	}
	defer rows.Close()
	var comments []InvoiceComment
	for rows.Next() {
		var comment InvoiceComment
		err := rows.Scan(
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
			return nil, 0, fmt.Errorf("failed to scan comment: %w", err)
		}
		comments = append(comments, comment)
	}

	return comments, total, nil
}

func (cr *InvoiceCommentsService) GetRepliesByCommentID(ctx context.Context, commentID string) ([]InvoiceComment, error) {
	const query = `
		SELECT id, invoice_id, user_id, commenter_name, commenter_email,
		       comment_text, is_verified, is_guest, parent_comment_id,
		       edited, edited_at, created_at, updated_at
		FROM invoice_comments
		WHERE parent_comment_id = $1
		ORDER BY created_at ASC
	`

	rows, err := cr.postgres.Query(ctx, query, commentID)
	if err != nil {
		return nil, fmt.Errorf("failed to query replies: %w", err)
	}
	defer rows.Close()

	var replies []InvoiceComment
	for rows.Next() {
		var reply InvoiceComment
		err := rows.Scan(
			&reply.ID,
			&reply.InvoiceID,
			&reply.UserID,
			&reply.CommenterName,
			&reply.CommenterEmail,
			&reply.CommentText,
			&reply.IsVerified,
			&reply.IsGuest,
			&reply.ParentCommentID,
			&reply.Edited,
			&reply.EditedAt,
			&reply.CreatedAt,
			&reply.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan reply: %w", err)
		}
		replies = append(replies, reply)
	}

	return replies, nil
}

func (cr *InvoiceCommentsService) UpdateCommentQuery(ctx context.Context, commentID, commentText string) error {
	const query = `
		UPDATE invoice_comments
		SET comment_text = $1, edited = TRUE, edited_at = NOW(), updated_at = NOW()
		WHERE id = $2
	`

	result, err := cr.postgres.Exec(ctx, query, commentText, commentID)
	if err != nil {
		return fmt.Errorf("failed to update comment: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("comment not found")
	}

	return nil
}

func (cr *InvoiceCommentsService) DeleteCommentQuery(ctx context.Context, commentID string) error {
	const query = `DELETE FROM invoice_comments WHERE id = $1`

	result, err := cr.postgres.Exec(ctx, query, commentID)
	if err != nil {
		return fmt.Errorf("failed to delete comment: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("comment not found")
	}

	return nil
}

func (cr *InvoiceCommentsService) GetCommentStats(ctx context.Context, invoiceID string) (*InvoiceCommentStats, error) {
	const query = `
		SELECT * FROM invoice_comment_stats
		WHERE invoice_id = $1
	`

	var stats InvoiceCommentStats
	err := cr.postgres.QueryRow(ctx, query, invoiceID).Scan(
		&stats.InvoiceID,
		&stats.TotalComments,
		&stats.VerifiedComments,
		&stats.GuestComments,
		&stats.TopLevelComments,
		&stats.ReplyComments,
		&stats.LatestCommentAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return &InvoiceCommentStats{
				InvoiceID: invoiceID,
			}, nil
		}
		return nil, fmt.Errorf("failed to get comment stats: %w", err)
	}

	return &stats, nil
}

func (cr *InvoiceCommentsService) CheckCommentOwnership(ctx context.Context, commentID, userID string) (bool, error) {
	const query = `SELECT EXISTS(SELECT 1 FROM invoice_comments WHERE id = $1 AND user_id = $2)`

	var exists bool
	err := cr.postgres.QueryRow(ctx, query, commentID, userID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check comment ownership: %w", err)
	}

	return exists, nil
}

func (cr *InvoiceCommentsService) CheckGuestCommentOwnership(ctx context.Context, commentID, email string) (bool, error) {
	const query = `SELECT EXISTS(SELECT 1 FROM invoice_comments WHERE id = $1 AND commenter_email = $2 AND is_guest = TRUE)`

	var exists bool
	err := cr.postgres.QueryRow(ctx, query, commentID, email).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check guest comment ownership: %w", err)
	}

	return exists, nil
}

func (cs *InvoiceCommentsService) CreateComment(ctx context.Context, dto CreateCommentDTO, userID *string) *utils.ApiResponse {
	_, err := cs.GetInvoiceByIDQuery(ctx, dto.InvoiceID)
	if err != nil {
		cs.log.Error("invoice not found", "invoice_id", dto.InvoiceID, "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusNotFound,
			Message:    "invoice not found",
		}
	}

	if dto.ParentID != "" {
		parentComment, err := cs.GetCommentByIDQuery(ctx, dto.ParentID)
		if err != nil {
			cs.log.Error("parent comment not found", "parent_id", dto.ParentID, "error", err)
			return &utils.ApiResponse{
				StatusCode: http.StatusNotFound,
				Message:    "parent comment not found",
			}
		}

		if parentComment.InvoiceID != dto.InvoiceID {
			return &utils.ApiResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "parent comment is not on the same invoice",
			}
		}
	}

	comment := &CreateCommentDTO{
		InvoiceID:     dto.InvoiceID,
		CommenterName: dto.CommenterName,
		CommentText:   dto.CommentText,
		ParentID:      dto.ParentID,
	}

	if userID != nil && *userID != "" {
		if _, err := uuid.Parse(*userID); err != nil {
			cs.log.Error("invalid user ID format", "user_id", *userID, "error", err)
			return &utils.ApiResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "invalid user ID format",
			}
		}

		user, err := cs.GetUserByIDQuery(ctx, *userID)
		if err != nil {
			cs.log.Error("failed to get user", "user_id", *userID, "error", err)
			return &utils.ApiResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    "failed to retrieve user information",
			}
		}

		comment.UserID = *userID
		comment.CommenterEmail = user.Email
		if user.FirstName != "" && user.LastName != "" {
			comment.CommenterName = fmt.Sprintf("%s %s", user.FirstName, user.LastName)
		}
	} else {
		// Guest commenter - leave UserID empty/default
		if dto.CommenterEmail == "" {
			return &utils.ApiResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "email is required for guest commenters",
			}
		}
		comment.CommenterEmail = dto.CommenterEmail
		// Don't set UserID at all for guests
	}

	createdComment, err := cs.CreateCommentQuery(ctx, *comment)
	if err != nil {
		cs.log.Error("failed to create comment", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "failed to create comment",
			Error:      err.Error(),
		}
	}

	cs.log.Info("comment created successfully",
		"comment_id", createdComment.ID,
		"invoice_id", dto.InvoiceID,
		"is_guest", createdComment.Guest,
	)

	return &utils.ApiResponse{
		StatusCode: http.StatusCreated,
		Message:    "comment created successfully",
		Data:       createdComment,
	}
}

func (cs *InvoiceCommentsService) GetComments(ctx context.Context, query GetCommentsQuery, currentUserID *string) *utils.ApiResponse {
	_, err := cs.GetInvoiceByIDQuery(ctx, query.InvoiceID)
	if err != nil {
		cs.log.Error("invoice not found", "invoice_id", query.InvoiceID, "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusNotFound,
			Message:    "invoice not found",
		}
	}

	if query.Page < 1 {
		query.Page = 1
	}
	if query.Limit < 1 || query.Limit > 100 {
		query.Limit = 20
	}

	offset := (query.Page - 1) * query.Limit

	sortOrder := "ASC"
	if query.SortOrder == "desc" {
		sortOrder = "DESC"
	}

	var parentIDPtr *string
	if query.ParentID != "" {
		parentIDPtr = &query.ParentID
	}

	comments, total, err := cs.GetCommentByInvoiceID(
		ctx,
		query.InvoiceID,
		parentIDPtr,
		query.Limit,
		offset,
		sortOrder,
	)
	if err != nil {
		cs.log.Error("failed to get comments", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "failed to retrieve comments",
			Error:      err.Error(),
		}
	}

	var commentResponses []CommentResponse
	for _, comment := range comments {
		response := cs.buildCommentResponse(ctx, comment, currentUserID)

		if !comment.ParentCommentID.Valid {
			replies, err := cs.GetRepliesByCommentID(ctx, comment.ID)
			if err != nil {
				cs.log.Warn("failed to get replies", "comment_id", comment.ID, "error", err)
			} else {
				for _, reply := range replies {
					response.Replies = append(response.Replies, cs.buildCommentResponse(ctx, reply, currentUserID))
				}
			}
		}

		commentResponses = append(commentResponses, response)
	}

	stats, err := cs.GetCommentStats(ctx, query.InvoiceID)
	if err != nil {
		cs.log.Warn("failed to get comment stats", "error", err)
		stats = &InvoiceCommentStats{InvoiceID: query.InvoiceID}
	}

	totalPages := (total + query.Limit - 1) / query.Limit

	response := GetCommentsResponse{
		Comments:   commentResponses,
		Stats:      *stats,
		Total:      total,
		Page:       query.Page,
		Limit:      query.Limit,
		TotalPages: totalPages,
	}

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "comments retrieved successfully",
		Data:       response,
	}
}

func (cs *InvoiceCommentsService) buildCommentResponse(ctx context.Context, comment InvoiceComment, currentUserID *string) CommentResponse {
	response := CommentResponse{
		InvoiceComment: comment,
		CanEdit:        false,
		CanDelete:      false,
	}

	if currentUserID != nil {
		if comment.UserID.Valid && comment.UserID.String == *currentUserID {
			response.CanEdit = true
			response.CanDelete = true
		}

		invoice, err := cs.GetInvoiceByIDQuery(ctx, comment.InvoiceID)
		if err == nil && invoice.UserID == *currentUserID {
			response.CanDelete = true
		}
	}

	return response
}

func (cs *InvoiceCommentsService) GetCommentByID(ctx context.Context, commentID string, currentUserID *string) *utils.ApiResponse {
	comment, err := cs.GetCommentByIDQuery(ctx, commentID)
	if err != nil {
		cs.log.Error("comment not found", "comment_id", commentID, "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusNotFound,
			Message:    "comment not found",
		}
	}
	response := cs.buildCommentResponse(ctx, *comment, currentUserID)

	replies, err := cs.GetRepliesByCommentID(ctx, comment.ID)
	if err != nil {
		cs.log.Warn("failed to get replies", "comment_id", comment.ID, "error", err)
	} else {
		for _, reply := range replies {
			response.Replies = append(response.Replies, cs.buildCommentResponse(ctx, reply, currentUserID))
		}
	}

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "comment retrieved successfully",
		Data:       response,
	}
}

func (cs *InvoiceCommentsService) UpdateComment(ctx context.Context, commentID string, dto UpdateCommentDTO, userID *string, guestEmail *string) *utils.ApiResponse {

	comment, err := cs.GetCommentByIDQuery(ctx, commentID)
	if err != nil {
		cs.log.Error("comment not found", "comment_id", commentID, "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusNotFound,
			Message:    "comment not found",
		}
	}

	canEdit := false
	if userID != nil && comment.UserID.Valid && comment.UserID.String == *userID {
		canEdit = true
	} else if guestEmail != nil && comment.IsGuest && comment.CommenterEmail == *guestEmail {
		canEdit = true
	}

	if !canEdit {
		return &utils.ApiResponse{
			StatusCode: http.StatusForbidden,
			Message:    "you don't have permission to edit this comment",
		}
	}

	err = cs.UpdateCommentQuery(ctx, commentID, dto.CommentText)
	if err != nil {
		cs.log.Error("failed to update comment", "comment_id", commentID, "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "failed to update comment",
			Error:      err.Error(),
		}
	}

	updatedComment, err := cs.GetCommentByIDQuery(ctx, commentID)
	if err != nil {
		cs.log.Error("failed to get updated comment", "comment_id", commentID, "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "failed to retrieve updated comment",
		}
	}

	cs.log.Info("comment updated successfully", "comment_id", commentID)

	response := cs.buildCommentResponse(ctx, *updatedComment, userID)

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "comment updated successfully",
		Data:       response,
	}
}

func (cs *InvoiceCommentsService) DeleteComment(ctx context.Context, commentID string, userID *string, guestEmail *string, isAdmin bool) *utils.ApiResponse {

	comment, err := cs.GetCommentByIDQuery(ctx, commentID)
	if err != nil {
		cs.log.Error("comment not found", "comment_id", commentID, "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusNotFound,
			Message:    "comment not found",
		}
	}

	canDelete := false
	if isAdmin {
		canDelete = true
	} else if userID != nil && comment.UserID.Valid && comment.UserID.String == *userID {
		canDelete = true
	} else if guestEmail != nil && comment.IsGuest && comment.CommenterEmail == *guestEmail {
		canDelete = true
	}

	if !canDelete && userID != nil {
		invoice, err := cs.GetInvoiceByIDQuery(ctx, comment.InvoiceID)
		if err == nil && invoice.UserID == *userID {
			canDelete = true
		}
	}

	if !canDelete {
		return &utils.ApiResponse{
			StatusCode: http.StatusForbidden,
			Message:    "you don't have permission to delete this comment",
		}
	}

	err = cs.DeleteCommentQuery(ctx, commentID)
	if err != nil {
		cs.log.Error("failed to delete comment", "comment_id", commentID, "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "failed to delete comment",
			Error:      err.Error(),
		}
	}

	cs.log.Info("comment deleted successfully", "comment_id", commentID)

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "comment deleted successfully",
	}
}
