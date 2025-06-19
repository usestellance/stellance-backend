package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/The-True-Hooha/stellance-backend.git/internal/user"
	"github.com/The-True-Hooha/stellance-backend.git/pkg/config"
	jwt_ "github.com/The-True-Hooha/stellance-backend.git/pkg/jwt"
	"github.com/The-True-Hooha/stellance-backend.git/pkg/utils"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

var (
	userCacheTime         = 1 * time.Hour
	ErrUserAlreadyExists  = errors.New("user already exists")
	ErrInvalidCredentials = errors.New("invalid credentials")
)

type AuthServiceConfig struct {
	log      *slog.Logger
	postgres *pgxpool.Pool
	redis    *redis.Client
}

func NewAuthService() *AuthServiceConfig {
	return &AuthServiceConfig{
		log:      config.GetAppContainer().Log,
		postgres: config.GetAppContainer().Postgres,
		redis:    config.GetAppContainer().Redis,
	}
}

func (config *AuthServiceConfig) CreateNewUser(ctx context.Context, dto AuthRequestDto) *utils.ApiResponse {
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

	const createNewUserQ = `INSERT INTO USERS(email, password) VALUES ($1, $2) RETURNING id, email, created_at`
	var user user.UserResponseDto
	err = tx.QueryRow(ctx, createNewUserQ, email, hash).Scan(&user.ID,
		&user.Email,
		&user.CreatedAt)
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
	// TODO: send email here
	userCache, _ := json.Marshal(user)
	err = redis.Set(ctx, user.ID, userCache, userCacheTime).Err()
	if err != nil {
		log.Error("failed to add new user record to cache", "error", err)
	}

	accessToken, err := jwt_.JwtTokenService().GenerateNewAccessToken(user.ID, user.Email)
	if err != nil {
		log.Error(fmt.Sprintf("error generating access token for user with Id =>> %s", user.ID), "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "service unavailable, kindly contact support",
			Error:      err.Error(),
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
				Data:       existingUser,
			}
		}
		accessToken, err := jwt_.JwtTokenService().GenerateNewAccessToken(existingUser.ID, existingUser.Email)
		if err != nil {
			config.log.Error(fmt.Sprintf("error generating access token for user with Id =>> %s", existingUser.ID), "error", err)
			return &utils.ApiResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    "service unavailable, kindly contact support",
				Error:      err.Error(),
			}
		}
		return &utils.ApiResponse{
			StatusCode: http.StatusOK,
			Message:    "login successful",
			Data: &AuthResponseDto{
				User:        *existingUser,
				AccessToken: accessToken,
				ExpiresIn:   time.Now().Add(1 * time.Hour).Unix(),
			},
		}
	}
	return &utils.ApiResponse{
		StatusCode: http.StatusForbidden,
		Message:    "invalid login credentials, contact support",
	}
}

func (config *AuthServiceConfig) ValidateEmail(ctx context.Context, email string) *utils.ApiResponse {
	const query = ` UPDATE USERS SET email_verified = TRUE, email_verified_at = NOW() WHERE email = $1`

	data, err := config.postgres.Exec(ctx, query, strings.ToLower(email))
	if err != nil {
		config.log.Error("failed to update user record to set email verified to true", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "contact support",
		}
	}

	if data.RowsAffected() == 0 {
		return &utils.ApiResponse{
			StatusCode: http.StatusForbidden,
			Message:    "Oops! It seems you haven't created your account yet. Credentials not found",
		}
	}
	config.log.Debug(fmt.Sprintf("email address for user %s verified successfully", email))
	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "Email address verified successfully",
	}
}

func (config *AuthServiceConfig) GenerateRefreshToken(ctx context.Context, accessToken string) *utils.ApiResponse {
	token, err := jwt_.JwtTokenService().GenerateRefreshToken(accessToken)
	if err != nil {
		return &utils.ApiResponse{
			StatusCode: http.StatusUnauthorized,
			Message:    "Invalid authorization request, contact support",
		}
	}

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "successful",
		Data: &AuthResponseDto{
			RefreshToken:       token,
			RefreshTokenExpiry: time.Now().Add(7 * 24 * time.Hour).Unix(),
		},
	}
}
