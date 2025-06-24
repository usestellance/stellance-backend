package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/The-True-Hooha/stellance-backend.git/internal/user"
	"github.com/The-True-Hooha/stellance-backend.git/mail"
	"github.com/The-True-Hooha/stellance-backend.git/pkg/config"
	jwt_ "github.com/The-True-Hooha/stellance-backend.git/pkg/jwt"
	"github.com/The-True-Hooha/stellance-backend.git/pkg/utils"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

var (
	userCacheTime         = 1 * time.Hour
	emailCacheTime        = 24 * 7 * time.Hour
	ErrUserAlreadyExists  = errors.New("user already exists")
	ErrInvalidCredentials = errors.New("invalid credentials")
)

type AuthServiceConfig struct {
	log      *slog.Logger
	postgres *pgxpool.Pool
	redis    *redis.Client
	jwt      *jwt_.JwtTokenServiceConfig
	mail     *mail.Mailer
}

func NewAuthService() *AuthServiceConfig {
	return &AuthServiceConfig{
		log:      config.GetAppContainer().Log,
		postgres: config.GetAppContainer().Postgres,
		redis:    config.GetAppContainer().Redis,
		jwt:      jwt_.JwtTokenService(),
		mail:     mail.NewMailer(),
	}
}

func (c *AuthServiceConfig) ClearRedis(ctx context.Context) *utils.ApiResponse {
	err := c.redis.FlushDB(ctx).Err()
	if err != nil {
		panic(err)
	}
	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "Cleared Successfully",
	}
}

func (config *AuthServiceConfig) CreateNewUser(ctx context.Context, dto AuthRequestDto, role user.UserRole) *utils.ApiResponse {
	db := config.postgres
	redis := config.redis
	log := config.log

	email := strings.ToLower(dto.Email)
	existingUser, err := user.NewUserService().FindUserByEmail(ctx, email)
	if err != nil {
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "service unavailable, kindly contact support",
			Error:      err.Error(),
		}
	}
	if existingUser != nil {
		return &utils.ApiResponse{
			StatusCode: http.StatusForbidden,
			Message:    fmt.Sprintf("user with this email %s already exist", email),
			Data:       nil,
		}
	}

	hash, err := utils.HashString(dto.Password)

	if err != nil {
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "service unavailable, kindly contact support",
			Error:      err.Error(),
		}
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "service unavailable, kindly contact support",
			Error:      err.Error(),
		}
	}
	defer tx.Rollback(ctx)

	const createNewUserQ string = `INSERT INTO USERS(email, password) VALUES ($1, $2) RETURNING id, email, created_at, is_active`
	var user user.UserProfileDto
	err = tx.QueryRow(ctx, createNewUserQ, email, hash).Scan(&user.ID,
		&user.Email,
		&user.CreatedAt,
		&user.IsActive,
	)
	if err != nil {
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "service unavailable, kindly contact support",
			Error:      err.Error(),
		}
	}

	if err = tx.Commit(ctx); err != nil {
		log.Error("failed to commit and save new user record", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "service unavailable, kindly contact support",
			Error:      err.Error(),
		}
	}
	userCache, _ := json.Marshal(user)
	err = redis.Set(ctx, user.ID, userCache, userCacheTime).Err()
	if err != nil {
		log.Error("failed to add new user record to cache", "error", err)
	}

	accessToken, err := config.jwt.GenerateNewAccessToken(user.ID, user.Email, string(role))
	if err != nil {
		log.Error(fmt.Sprintf("error generating access token for user with Id =>> %s", user.ID), "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "service unavailable, kindly contact support",
			Error:      err.Error(),
		}
	}
	err = config.GenerateAndSendEmail(ctx, email, user.ID, config.log)
	if err != nil {
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Server currently unavailable",
		}
	}

	log.Debug(fmt.Sprintf("new user with ID %s created successfully", user.ID))
	return &utils.ApiResponse{
		StatusCode: http.StatusCreated,
		Message:    "account has successfully been created, and email sent for verification",
		Data: &AuthResponseDto{
			User:        user,
			AccessToken: accessToken,
			ExpiresIn:   time.Now().Add(1 * time.Hour).Unix(),
		},
	}
}

func (config *AuthServiceConfig) Login(ctx context.Context, dto AuthRequestDto) *utils.ApiResponse {
	email := strings.ToLower(dto.Email)
	existingUser, err := user.NewUserService().FindUserByEmail(ctx, email)
	if err != nil {
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "service unavailable, kindly contact support",
			Error:      err.Error(),
		}
	}
	if existingUser != nil {
		err = utils.CompareHash(existingUser.Password, dto.Password)
		if err != nil {
			return &utils.ApiResponse{
				StatusCode: http.StatusForbidden,
				Message:    "invalid login credentials, contact support",
			}
		}

		if !existingUser.EmailVerified {
			err = config.GenerateAndSendEmail(ctx, email, existingUser.ID, config.log)
			if err != nil {
				return &utils.ApiResponse{
					StatusCode: http.StatusInternalServerError,
					Message:    "Server currently unavailable",
				}
			}
		}
		accessToken, err := config.jwt.GenerateNewAccessToken(existingUser.ID, existingUser.Email, string(existingUser.Role))
		if err != nil {
			config.log.Error(fmt.Sprintf("error generating access token for user with Id =>> %s", existingUser.ID), "error", err)
			return &utils.ApiResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    "service unavailable, kindly contact support",
				Error:      err.Error(),
			}
		}
		existingUser.Password = ""
		return &utils.ApiResponse{
			StatusCode: http.StatusOK,
			Message:    "Login successful! A new email has been sent to your email address please verify your email",
			Data: &AuthResponseDto{
				User:            *existingUser,
				AccessToken:     accessToken,
				ExpiresIn:       time.Now().Add(1 * time.Hour).Unix(),
				EmailVerified:   existingUser.EmailVerified,
				ProfileComplete: existingUser.FirstName != nil && existingUser.LastName != nil,
			},
		}
	}
	return &utils.ApiResponse{
		StatusCode: http.StatusForbidden,
		Message:    "invalid login credentials, contact support",
	}
}

func (config *AuthServiceConfig) ValidateEmail(ctx context.Context, token, email_ string) *utils.ApiResponse {
	email, err := config.redis.Get(ctx, fmt.Sprintf("email_%s", token)).Result()

	if err != nil {
		err = config.GenerateAndSendEmail(ctx, email_, token, config.log)
		if err != nil {
			return &utils.ApiResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    "Server currently unavailable",
			}
		}
		return &utils.ApiResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "your email verification link has expired, a new email has been sent to you",
		}
	}
	email = strings.ToLower(email)
	tx, err := config.postgres.Begin(ctx)
	if err != nil {
		config.log.Error("failed to begin transaction", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to process request. Please try again.",
		}
	}
	defer tx.Rollback(ctx)

	var user struct {
		ID            string
		EmailVerified bool
	}

	const checkQuery string = `
		SELECT id, email_verified 
		FROM users 
		WHERE email = $1
		FOR UPDATE
	`

	err = tx.QueryRow(ctx, checkQuery, email).Scan(
		&user.ID,
		&user.EmailVerified,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return &utils.ApiResponse{
				StatusCode: http.StatusNotFound,
				Message:    "No account found with this email address. Please sign up first.",
			}
		}
		config.log.Error("failed to fetch user", "error", err, "email", email)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to validate email. Please try again later.",
		}
	}

	if user.EmailVerified {
		return &utils.ApiResponse{
			StatusCode: http.StatusOK,
			Message:    "Your email is already verified. You can log in to your account.",
		}
	}

	const updateQuery = `
		UPDATE users 
		SET email_verified = TRUE, 
		    email_verified_at = NOW()
		WHERE id = $1
	`
	_, err = tx.Exec(ctx, updateQuery, user.ID)
	if err != nil {
		config.log.Info("failed to update verification", "error", err, "user_id", user.ID)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to verify email. Please contact support.",
		}
	}

	if err = tx.Commit(ctx); err != nil {
		config.log.Error("failed to commit transaction", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to complete verification. Please try again.",
		}
	}

	config.log.Info("email verified successfully",
		"email", email,
		"user_id", user.ID,
	)

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    fmt.Sprintf("Welcome %s! Your email has been verified successfully.", email),
	}
}

func (config *AuthServiceConfig) GenerateRefreshToken(ctx context.Context, accessToken string) *utils.ApiResponse {
	token, err := config.jwt.GenerateRefreshToken(accessToken)
	if err != nil {
		return &utils.ApiResponse{
			StatusCode: http.StatusUnauthorized,
			Message:    "Invalid authorization request, contact support",
		}
	}

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "successful",
		Data: map[string]interface{}{
			"refresh_token":        token,
			"refresh_token_expiry": time.Now().Add(7 * 24 * time.Hour).Unix(),
		},
	}
}

func (c *AuthServiceConfig) GenerateAndSendEmail(ctx context.Context, email, user_id string, log *slog.Logger) error {
	emailToken, err := utils.GenerateShortURL(fmt.Sprintf("%s%s", email, user_id), log)
	if err != nil {
		log.Error(fmt.Sprintf("error failed to generate email token for userId =>> %s", user_id), "error", err)
		return err
	}
	err = c.redis.Set(ctx, fmt.Sprintf("email_%s", emailToken), email, emailCacheTime).Err()
	if err != nil {
		log.Error("failed to add email token to redis", "error", err)
		return err
	}
	go func() {
		ee := url.QueryEscape(email)
		eu := url.QueryEscape(emailToken)
		email_url := fmt.Sprintf("https://usestellance.com/sign-up/verify-email?email=%s&token=%s", ee, eu)
		err := c.mail.SendVerificationEmail(email, email_url)
		if err != nil {
			log.Warn("error sending email to user", "error", err)
		}
	}()
	return nil
}

func (c *AuthServiceConfig) ResendEmail(ctx context.Context, email string) *utils.ApiResponse {
	token, err := utils.GenerateSecureToken(16)
	if err != nil {
		c.log.Info("error trying to generate random string")
		token = "idnaskfnakfnpdmffadfaerfwfgsgsgwsbesnh"
	}

	err = c.GenerateAndSendEmail(ctx, email, token, c.log)

	if err != nil {
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Server is currently unavailable",
		}
	}
	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "Email has been sent",
	}
}
