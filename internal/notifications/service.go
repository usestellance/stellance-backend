package notifications

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"math"
	"net/http"

	"github.com/The-True-Hooha/stellance-backend/mail"
	"github.com/The-True-Hooha/stellance-backend/pkg/config"
	jwt_ "github.com/The-True-Hooha/stellance-backend/pkg/jwt"
	"github.com/The-True-Hooha/stellance-backend/pkg/utils"
	"github.com/jackc/pgx"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type NotificationService struct {
	log      *slog.Logger
	postgres *pgxpool.Pool
	redis    *redis.Client
	jwt      *jwt_.JwtTokenServiceConfig
	mail     *mail.Mailer
}

func NewNotificationService() *NotificationService {
	return &NotificationService{
		log:      config.GetAppContainer().Log,
		postgres: config.GetAppContainer().Postgres,
		redis:    config.GetAppContainer().Redis,
		jwt:      jwt_.JwtTokenService(),
		mail:     mail.NewMailer(),
	}
}

func (ns *NotificationService) CreateNewNotification(ctx context.Context, data CreateNotificationDto) (string, error) {
	tx, err := ns.postgres.Begin(ctx)
	if err != nil {
		ns.log.Error("failed to begin transaction", "error", err)
		return "", err
	}
	defer tx.Rollback(ctx)

	var notificationID string
	const query = `
	INSERT INTO notifications (user_id, title, body)
	VALUES ($1, $2, $3)
	RETURNING id
	`

	err = tx.QueryRow(ctx, query, data.UserId, data.Title, data.Body).Scan(&notificationID)
	if err != nil {
		ns.log.Error("failed to create new notification record", "error", err)
		return "", err
	}

	if err := tx.Commit(ctx); err != nil {
		ns.log.Error("failed to commit transaction", "error", err)
		return "", err
	}

	return notificationID, nil
}

func (ns *NotificationService) UpdateNotificationViewStatus(ctx context.Context, notificationID, userId string, viewed bool) *utils.ApiResponse {
	tx, err := ns.postgres.Begin(ctx)
	if err != nil {
		ns.log.Error("failed to begin transaction", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		}
	}
	defer tx.Rollback(ctx)
	var query string
	if viewed {
		query = `
			UPDATE notifications
			SET viewed = true,
				viewed_at = NOW(),
				updated_at = NOW()
			WHERE id = $1
			AND user_id = $2
		`
	} else {
		query = `
			UPDATE notifications
			SET viewed = false,
				viewed_at = NULL,
				updated_at = NOW()
			WHERE id = $1
			AND user_id = $2
		`
	}

	_, err = tx.Exec(ctx, query, notificationID, userId)
	if err != nil {
		ns.log.Error("failed to update notification view status", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		}
	}

	if err := tx.Commit(ctx); err != nil {
		ns.log.Error("failed to commit transaction", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Internal server error, please contact support",
		}
	}

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "notification updated successfully",
	}
}

func (ns *NotificationService) GetUserNotifications(ctx context.Context, userID string, dto GetNotificationsQuery) *utils.ApiResponse {
	offset := (dto.Page - 1) * dto.Count
	const countQuery = `
		SELECT
			COUNT(*) FILTER (WHERE viewed = false) AS unread_count,
			COUNT(*) FILTER (WHERE viewed = true) AS read_count,
			COUNT(*) AS total_count
		FROM notifications
		WHERE user_id = $1
	`

	var unreadCount, readCount, totalCount int
	err := ns.postgres.QueryRow(ctx, countQuery, userID).Scan(&unreadCount, &readCount, &totalCount)
	if err != nil {
		ns.log.Error("failed to query database for notifications")
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Service unreachable",
		}
	}

	query := `
		SELECT id, title, body, viewed, viewed_at
		FROM notifications
		WHERE user_id = $1
	`

	args := []interface{}{userID}
	argCount := 1

	if dto.Viewed != nil {
		argCount++
		query += fmt.Sprintf(" AND VIEWED = $%d", argCount)
		args = append(args, *dto.Viewed)
	}

	query += fmt.Sprintf(" ORDER BY created_at %s", dto.OrderBy)

	argCount++
	query += fmt.Sprintf(" LIMIT $%d", argCount)
	args = append(args, dto.Count)

	argCount++
	query += fmt.Sprintf(" OFFSET $%d", argCount)
	args = append(args, offset)

	rows, err := ns.postgres.Query(ctx, query, args...)
	if err != nil {
		ns.log.Error("failed to query for notifications")
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Service unreachable",
		}
	}
	defer rows.Close()
	
	var result []Notification
	for rows.Next() {
		var notif Notification
		var viewedAt sql.NullTime
		
		err := rows.Scan(&notif.Id, &notif.Title, &notif.Body, &notif.Viewed, &viewedAt)
		if err != nil {
			ns.log.Error("failed to scan into results for notifications")
			return &utils.ApiResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    "Service unreachable",
			}
		}

		if viewedAt.Valid {
			t := viewedAt.Time
			notif.ViewedAt = &t
		} else {
			notif.ViewedAt = nil
		}
		result = append(result, notif)
	}

	if err = rows.Err(); err != nil {
		ns.log.Error("row iteration error", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Service unreachable",
		}
	}

	filteredCount := totalCount
	if dto.Viewed != nil {
		if *dto.Viewed {
			filteredCount = readCount
		}else{
			filteredCount = unreadCount
		}
	}

	totalPages := int(math.Ceil((float64(filteredCount) / float64(dto.Count))))

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "successful",
		Data: GetNotificationResponse{
			Notifications: result,
			Meta: PaginationMeta{
				Page: dto.Page,
				PageCount: len(result),
				UnreadCount: unreadCount,
				ReadCount: readCount,
				TotalPages: totalPages,

			},
		},
	}
}

func (ns *NotificationService) GetNotificationByID(ctx context.Context, id, userId string) *utils.ApiResponse {
	const query = `
		SELECT id, title, body, viewed, viewed_at
		FROM notifications
		WHERE id = $1 AND user_id = $2
	`

	var notif Notification

	err := ns.postgres.QueryRow(ctx, query, id, userId).Scan(
		&notif.Id,
		&notif.Title,
		&notif.Body,
		&notif.Viewed,
		&notif.ViewedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			ns.log.Debug("notification does not exist", "id", id)
			return &utils.ApiResponse{
				StatusCode: http.StatusNotFound,
				Message:    "Unavailable",
			}
		}
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Service Unreachable",
		}
	}

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "successful",
		Data:       &notif,
	}

}

func (ns *NotificationService) DeleteNotificationByID(ctx context.Context, id, userId string) *utils.ApiResponse {
	const query = `
		DELETE FROM notifications
		WHERE id = $1
		AND user_id = $2
	`

	cmd, err := ns.postgres.Exec(ctx, query, id, userId)
	if err != nil {
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Service unreachable",
		}
	}

	if cmd.RowsAffected() == 0 {
		return &utils.ApiResponse{
			StatusCode: http.StatusNotFound,
			Message:    "Unavailable",
		}
	}

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "successful",
	}
}
