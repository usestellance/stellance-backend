package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"crypto/rand"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/The-True-Hooha/stellance-backend/internal/user"
	"github.com/The-True-Hooha/stellance-backend/internal/wallet"
	"github.com/The-True-Hooha/stellance-backend/mail"
	"github.com/The-True-Hooha/stellance-backend/pkg/config"
	jwt_ "github.com/The-True-Hooha/stellance-backend/pkg/jwt"
	"github.com/The-True-Hooha/stellance-backend/pkg/utils"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

var (
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
		config.log.Error("error fetching user from database", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "service unavailable, kindly contact support",
		}
	}
	if existingUser == nil {
		return &utils.ApiResponse{
			StatusCode: http.StatusNotFound,
			Message:    "invalid login credentials, contact support",
		}
	} else {
		err = utils.CompareHash(*existingUser.Password, dto.Password)
		if err != nil {
			return &utils.ApiResponse{
				StatusCode: http.StatusForbidden,
				Message:    "invalid login credentials, contact support",
			}
		}
		var address sql.NullString
		var walletId sql.NullString

		const q string = `
			SELECT id, address
			FROM wallets 
			WHERE user_id = $1 AND is_primary = true AND is_active = true
		`
		err = config.postgres.QueryRow(ctx, q, existingUser.ID).Scan(&walletId, &address)
		if err != nil {
			if err == pgx.ErrNoRows {
				config.log.Debug("no wallet details found for user", "userId", existingUser.ID)
			}
			config.log.Error("failed to fetch wallet", "error", err, "userId", existingUser.ID)
		}

		userCopy := &user.User{
			Id:            existingUser.ID,
			FirstName:     existingUser.FirstName,
			LastName:      existingUser.LastName,
			Email:         existingUser.Email,
			BusinessName:  existingUser.BusinessName,
			PhoneNumber:   existingUser.PhoneNumber,
			Country:       existingUser.Country,
			EmailVerified: &existingUser.EmailVerified,
		}

		if walletId.Valid && address.Valid {
			userCopy.Wallet = &user.UserWallet{
				Id:      &walletId.String,
				Address: &address.String,
			}
			userCopy.Wallet.Balance = &user.WalletBalance{}
			bal, _ := wallet.NewWalletService().GetAccountBalance(ctx, address.String, walletId.String)
			userCopy.Wallet.Balance.USDC = &bal.USDC
			userCopy.Wallet.Balance.XLM = &bal.XLM
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

		profileComplete := existingUser.FirstName != nil && existingUser.LastName != nil

		return &utils.ApiResponse{
			StatusCode: http.StatusOK,
			Message:    "Login successful",
			Data: &AuthLoginResponseDto{
				AccessToken:     accessToken,
				ExpiresIn:       time.Now().Add(1 * time.Hour).Unix(),
				EmailVerified:   existingUser.EmailVerified,
				ProfileComplete: profileComplete,
				User:            userCopy,
			},
		}

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

	var user_ struct {
		ID            string
		EmailVerified bool
		role          string
	}

	const checkQuery string = `
		SELECT id, email_verified, permission
		FROM users 
		WHERE email = $1
		FOR UPDATE
	`

	err = tx.QueryRow(ctx, checkQuery, email).Scan(
		&user_.ID,
		&user_.EmailVerified,
		&user_.role,
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

	if user_.EmailVerified {
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
	_, err = tx.Exec(ctx, updateQuery, user_.ID)
	if err != nil {
		config.log.Info("failed to update verification", "error", err, "user_id", user_.ID)
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
		"user_id", user_.ID,
	)

	accessToken, err := config.jwt.GenerateNewAccessToken(user_.ID, email, user_.role)
	if err != nil {
		config.log.Error(fmt.Sprintf("error generating access token for user with Id =>> %s", user_.ID), "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "service unavailable, kindly contact support",
			Error:      err.Error(),
		}
	}

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    fmt.Sprintf("Welcome %s! Your email has been verified successfully.", email),
		Data: &AuthLoginResponseDto{
			AccessToken:     accessToken,
			ExpiresIn:       time.Now().Add(1 * time.Hour).Unix(),
			EmailVerified:   true,
			ProfileComplete: false,
		},
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
		StatusCode: http.StatusCreated,
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
		email_url := fmt.Sprintf("https://usestellance.com/auth/sign-up/verify-email?email=%s&token=%s", ee, eu)
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

func (as *AuthServiceConfig) RequestPasswordReset(ctx context.Context, email string) *utils.ApiResponse {
	email = strings.ToLower(email)
	existingUser, err := user.NewUserService().FindUserByEmail(ctx, email)
	if err != nil {
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "service unavailable, kindly contact support",
			Error:      err.Error(),
		}
	}

	if existingUser == nil {
		return &utils.ApiResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("there's no account with this email '%s', kindly contact support if you think this is unusual", email),
		}
	}
	otp := as.GenerateOTP()
	data := map[string]interface{}{
		"email": email,
		"otp":   otp,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Sever is currently unavailable at the moment, kindly contact support",
		}
	}
	err = as.redis.Set(ctx, fmt.Sprintf("email_otp_%s", email), jsonData, emailCacheTime).Err()
	if err != nil {
		as.log.Error("failed to add email token to redis", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Sever is currently unavailable at the moment, kindly contact support",
		}
	}

	go func() {
		ee := url.QueryEscape(email)
		email_url := fmt.Sprintf("https://usestellance.com/auth/reset-password?email=%s", ee)
		err := as.mail.SendResetEmail(email, email_url, otp)
		if err != nil {
			as.log.Warn("error sending reset email to user", "error", err)
		}
	}()
	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "Reset Email has successfully been sent",
	}
}

func (m *AuthServiceConfig) GenerateOTP() string {
	otp := ""
	for i := 0; i < 6; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			panic("failed to generate secure random number for OTP")
		}
		otp += n.String()
	}
	return otp
}

func (as *AuthServiceConfig) ChangeUserPassword(ctx context.Context, dto ChangePasswordDTO, userID string) *utils.ApiResponse {

	const getPasswordQuery = `SELECT password from users where id = $1 AND is_active = true`
	var passwordH string
	var email string

	err := as.postgres.QueryRow(ctx, getPasswordQuery, userID).Scan(&passwordH)
	if err != nil {
		if err == pgx.ErrNoRows {
			return &utils.ApiResponse{
				StatusCode: http.StatusNotFound,
				Message:    "invalid request, contact support",
			}
		}
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to get user profile details",
		}
	}

	err = utils.CompareHash(passwordH, dto.OldPassword)
	if err != nil {
		return &utils.ApiResponse{
			StatusCode: http.StatusForbidden,
			Message:    "invalid request, credentials does not match",
		}
	}

	const q = `
		UPDATE users 
			SET password = $1, updated_at = NOW()
			WHERE id = $2 AND is_active = true
			RETURNING email
	`

	hash, err := utils.HashString(dto.NewPassword)
	if err != nil {
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "service unavailable, kindly contact support",
			Error:      err.Error(),
		}
	}

	err = as.postgres.QueryRow(ctx, q, hash, userID).Scan(&email)
	if err != nil {
		if err == pgx.ErrNoRows {
			return &utils.ApiResponse{
				StatusCode: http.StatusNotFound,
				Message:    "invalid request, contact support",
			}
		}
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to get user profile details",
		}
	}

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "password changed successfully",
		Data: map[string]interface{}{
			"changedAt": time.Now(),
		},
	}
}

func (as *AuthServiceConfig) ResetPassword(ctx context.Context, dto ResetPasswordDto) *utils.ApiResponse {
	data, err := as.redis.Get(ctx, fmt.Sprintf("email_otp_%s", strings.ToLower(dto.Email))).Result()
	if err != nil {
		return as.RequestPasswordReset(ctx, dto.Email)
	}
	var cached struct {
		Email string `json:"email"`
		OTP   string `json:"otp"`
	}

	if err := json.Unmarshal([]byte(data), &cached); err != nil {
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Sever is currently unavailable at the moment, kindly contact support",
		}
	}

	if cached.OTP != dto.Otp {
		return &utils.ApiResponse{
			StatusCode: http.StatusForbidden,
			Message:    "OTP data in missing or incorrect",
		}
	}

	if dto.Password != dto.ConfirmPassword {
		return &utils.ApiResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "Your password and confirm password does not match",
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
	tx, err := as.postgres.Begin(ctx)
	if err != nil {
		as.log.Error("failed to begin transaction", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to process request",
		}
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	const query = `UPDATE users SET password = $1 WHERE email = $2`

	_, err = tx.Exec(ctx, query, hash, cached.Email)
	if err != nil {
		as.log.Error("failed to update user password", "error", err, "email", cached.Email)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to update user password",
		}
	}

	if err := tx.Commit(ctx); err != nil {
		as.log.Error("failed to commit transaction", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to finalize update",
		}
	}

	as.log.Info("user password updated successfully", "email", cached.Email)

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "Password updated successfully",
	}
}

func (as *AuthServiceConfig) HandleSocialAuth(ctx context.Context, dto ProviderLogin) *utils.ApiResponse {
	dto.Email = strings.ToLower(strings.TrimSpace(dto.Email))

	existingUser, err := user.NewUserService().FindUserByEmail(ctx, dto.Email)
	if err != nil {
		as.log.Error("error getting user from database", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "service unavailable, kindly contact support",
		}
	}

	if existingUser != nil {
		if existingUser.AuthType == "password" {
			as.log.Error(fmt.Sprintf("error logging in, user must log with correct auth type =>> %s, user email ===> %s", existingUser.AuthType, existingUser.Email))
			return &utils.ApiResponse{
				StatusCode: http.StatusForbidden,
				Message:    "Kindly login with your password, or contact support",
			}
		}

		if existingUser.AuthType != "password" && existingUser.ProviderID != nil {

			accessToken, err := as.jwt.GenerateNewAccessToken(existingUser.ID, existingUser.Email, string("user"))
			if err != nil {
				as.log.Error(fmt.Sprintf("error generating access token for user with Id =>> %s", existingUser.ID), "error", err)
				return &utils.ApiResponse{
					StatusCode: http.StatusInternalServerError,
					Message:    "service unavailable, kindly contact support",
				}
			}

			profileComplete := existingUser.FirstName != nil && existingUser.LastName != nil
			return &utils.ApiResponse{
				StatusCode: http.StatusOK,
				Message:    "login successful",
				Data: &AuthLoginResponseDto{
					EmailVerified:   existingUser.EmailVerified,
					ProfileComplete: profileComplete,
					ExpiresIn:       time.Now().Add(1 * time.Hour).Unix(),
					AccessToken:     accessToken,
					User: &user.User{
						Id:    existingUser.ID,
						Email: existingUser.Email,
					},
				},
			}
		}
	}

	tx, err := as.postgres.Begin(ctx)
	if err != nil {
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "service unavailable, kindly contact support",
		}
	}
	defer tx.Rollback(ctx)

	newUser := &user.User{}

	const createUserQ = `
		INSERT INTO users (email, provider_id, auth_type) 
		VALUES ($1, $2, $3) 
		RETURNING id, email
	`

	err = tx.QueryRow(ctx, createUserQ, dto.Email, dto.ProviderID, "google").Scan(
		&newUser.Id,
		&newUser.Email,
	)

	if err != nil {
		as.log.Error("failed to create new user", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "service unavailable, kindly contact support",
		}
	}

	if err = tx.Commit(ctx); err != nil {
		as.log.Error("failed to commit and save new user record", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "service unavailable, kindly contact support",
		}
	}

	accessToken, err := as.jwt.GenerateNewAccessToken(newUser.Id, newUser.Email, string("user"))
	if err != nil {
		as.log.Error(fmt.Sprintf("error generating access token for user with Id =>> %s", newUser.Id), "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "service unavailable, kindly contact support",
		}
	}

	err = as.GenerateAndSendEmail(ctx, dto.Email, newUser.Id, as.log)
	if err != nil {
		as.log.Error("failed to send verification email", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Server currently unavailable",
		}
	}

	as.log.Debug(fmt.Sprintf("new user with ID %s created successfully", newUser.Id))

	return &utils.ApiResponse{
		StatusCode: http.StatusCreated,
		Message:    "account has successfully been created, and email sent for verification",
		Data: map[string]interface{}{
			"access_token": accessToken,
			"expires_in":   time.Now().Add(1 * time.Hour).Unix(),
		},
	}
}
