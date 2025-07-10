package user

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/The-True-Hooha/stellance-backend.git/pkg/config"
	jwt_ "github.com/The-True-Hooha/stellance-backend.git/pkg/jwt"
	"github.com/The-True-Hooha/stellance-backend.git/pkg/utils"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type UserService struct {
	log        *slog.Logger
	postgres   *pgxpool.Pool
	redis      *redis.Client
	jwtService *jwt_.JwtTokenServiceConfig
}

func NewUserService() *UserService {
	return &UserService{
		log:        config.GetAppContainer().Log,
		postgres:   config.GetAppContainer().Postgres,
		redis:      config.GetAppContainer().Redis,
		jwtService: jwt_.JwtTokenService(),
	}
}

func (s *UserService) FindUserByEmail(ctx context.Context, email string) (*UserProfileDto, error) {
	email = strings.ToLower(email)
	const query string = `
		SELECT id, email, password, permission, email_verified, first_name, last_name, country, business_name, phone_number
		FROM users
		WHERE email = $1
	`
	var business_name sql.NullString
	var phone_number sql.NullString
	var user UserProfileDto
	var country_ sql.NullString
	err := s.postgres.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.Password,
		&user.Role,
		&user.EmailVerified,
		&user.FirstName,
		&user.LastName,
		&country_,
		&business_name,
		&phone_number,
	)
	if business_name.Valid {
		user.BusinessName = &business_name.String
	}
	if phone_number.Valid {
		user.PhoneNumber = &phone_number.String
	}
	if country_.Valid {
		user.Country = country_.String
	}

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		s.log.Error("failed to get user from postgres", "error", err)
		return nil, err
	}
	return &user, nil
}

func (s *UserService) CheckUserVerification(ctx context.Context, email string) (*UserProfileDto, error) {
	email = strings.ToLower(email)
	const query = `SELECT id, email, email_verified, email_verified_at FROM USERS WHERE email = $1`
	var user UserProfileDto
	err := s.postgres.QueryRow(ctx, query, email).Scan(&user.ID, &user.Email, &user.EmailVerified, &user.EmailVerifiedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		s.log.Error("failed to get user from postgres", "error", err)
		return nil, err
	}
	return &user, nil
}

func (s *UserService) CompleteUserProfile(ctx context.Context, email string, dto CompleteProfileRequestDto) *utils.ApiResponse {
	log := s.log
	email = strings.ToLower(email)

	tx, err := s.postgres.Begin(ctx)
	if err != nil {
		log.Error("failed to begin transaction", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to process request",
		}
	}
	defer tx.Rollback(ctx)

	var user struct {
		ID            string
		Email         string
		EmailVerified bool
		FirstName     *string
	}

	const checkQuery = `
		SELECT id, email, email_verified, first_name 
		FROM users
		WHERE email = $1 
		FOR UPDATE
	`

	err = tx.QueryRow(ctx, checkQuery, email).Scan(
		&user.ID,
		&user.Email,
		&user.EmailVerified,
		&user.FirstName,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return &utils.ApiResponse{
				StatusCode: http.StatusNotFound,
				Message:    "User not found",
			}
		}
		log.Error("failed to fetch user to complete profile", "error", err, "email", email)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to fetch user details",
		}
	}

	if user.FirstName != nil && *user.FirstName != "" {
		return &utils.ApiResponse{
			StatusCode: http.StatusConflict,
			Message:    "Profile already completed",
		}
	}

	if !user.EmailVerified {
		return &utils.ApiResponse{
			StatusCode: http.StatusForbidden,
			Message:    "Please verify your email first",
		}
	}
	updateQuery := `
		UPDATE users 
		SET 
			first_name = $2,
			last_name = $3,
			business_name = NULLIF($4, ''),
			phone_number = NULLIF($5, ''),
			country = NULLIF($6, ''),
			updated_at = NOW()
		WHERE id = $1
		RETURNING 
			id, email, first_name, last_name, 
			business_name, phone_number, country, 
			email_verified, created_at, updated_at
	`

	var updatedUser struct {
		ID            string
		Email         string
		FirstName     string
		LastName      string
		BusinessName  *string
		PhoneNumber   *string
		Country       *string
		EmailVerified bool
		CreatedAt     time.Time
		UpdatedAt     time.Time
	}

	err = tx.QueryRow(ctx, updateQuery,
		user.ID,
		dto.FirstName,
		dto.LastName,
		dto.BusinessName,
		dto.PhoneNumber,
		dto.Country,
	).Scan(
		&updatedUser.ID,
		&updatedUser.Email,
		&updatedUser.FirstName,
		&updatedUser.LastName,
		&updatedUser.BusinessName,
		&updatedUser.PhoneNumber,
		&updatedUser.Country,
		&updatedUser.EmailVerified,
		&updatedUser.CreatedAt,
		&updatedUser.UpdatedAt,
	)
	if err != nil {
		log.Error("failed to update user profile", "error", err, "user_id", user.ID)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to update profile",
		}
	}

	if err = tx.Commit(ctx); err != nil {
		log.Error("failed to commit transaction", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to complete profile update",
		}
	}

	log.Info("user profile completed successfully",
		"user_id", user.ID,
		"email", updatedUser.Email,
	)

	response := map[string]interface{}{
		"id":                updatedUser.ID,
		"email":             updatedUser.Email,
		"first_name":        updatedUser.FirstName,
		"last_name":         updatedUser.LastName,
		"business_name":     updatedUser.BusinessName,
		"phone_number":      updatedUser.PhoneNumber,
		"country":           updatedUser.Country,
		"profile_completed": true,
	}

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "Profile completed successfully",
		Data:       response,
	}
}

func (s *UserService) GetProfileByID(ctx context.Context, userID string) *utils.ApiResponse {
	log := s.log

	var (
		address      sql.NullString
		usdcBalance  sql.NullFloat64
		xlmBalance   sql.NullFloat64
		firstName    sql.NullString
		lastName     sql.NullString
		businessName sql.NullString
		phoneNumber  sql.NullString
	)

	const query = `
		SELECT 
		u.id, u.email, u.first_name, u.last_name, u.business_name,
		u.phone_number, u.country, u.email_verified, u.is_active,
		u.created_at, u.updated_at,
		w.address, w.usdc_balance, w.xlm_balance
		FROM users u
		LEFT JOIN wallets w ON w.user_id = u.id AND w.is_primary = true AND w.is_active = true
		WHERE u.id = $1 AND u.is_active = true
	`

	var profile UserProfileDto
	err := s.postgres.QueryRow(ctx, query, userID).Scan(
		&profile.ID,
		&profile.Email,
		&firstName,
		&lastName,
		&businessName,
		&phoneNumber,
		&profile.Country,
		&profile.EmailVerified,
		&profile.IsActive,
		&profile.CreatedAt,
		&profile.UpdatedAt,
		&address,
		&usdcBalance,
		&xlmBalance,
	)

	if firstName.Valid {
		profile.FirstName = &firstName.String
	}

	if lastName.Valid {
		profile.LastName = &lastName.String
	}

	if businessName.Valid {
		profile.BusinessName = &businessName.String
	}

	if phoneNumber.Valid {
		profile.PhoneNumber = &phoneNumber.String
	}

	if err != nil {
		if err == pgx.ErrNoRows {
			return &utils.ApiResponse{
				StatusCode: http.StatusNotFound,
				Message:    "User profile not found",
			}
		}
		log.Error("failed to fetch user profile", "error", err, "user_id", userID)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to fetch profile",
		}
	}

	profileComplete := profile.FirstName != nil && profile.LastName != nil

	wallet := &UserWallet{
		Balance: &WalletBalance{},
	}

	if address.Valid || usdcBalance.Valid || xlmBalance.Valid {
		wallet = &UserWallet{
			Address: utils.NullStringPtr(address),
			Balance: &WalletBalance{
				USDC: utils.NullFloatPtr(usdcBalance),
				XLM:  utils.NullFloatPtr(xlmBalance),
			},
		}
	}

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "Profile fetched successfully",
		Data: map[string]interface{}{
			"profile":          profile,
			"profile_complete": profileComplete,
			"email_verified":   !profile.EmailVerified,
			"wallet":           wallet,
		},
	}
}

func (s *UserService) UpdateProfile(ctx context.Context, userID string, dto UpdateProfileDto) *utils.ApiResponse {
	log := s.log

	tx, err := s.postgres.Begin(ctx)
	if err != nil {
		log.Error("failed to begin transaction", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to process request",
		}
	}
	defer tx.Rollback(ctx)

	var currentProfile struct {
		FirstName string
		LastName  string
	}

	const checkQuery = `
		SELECT first_name, last_name 
		FROM users 
		WHERE id = $1 AND is_active = true
		FOR UPDATE
	`

	err = tx.QueryRow(ctx, checkQuery, userID).Scan(
		&currentProfile.FirstName,
		&currentProfile.LastName,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return &utils.ApiResponse{
				StatusCode: http.StatusNotFound,
				Message:    "User not found",
			}
		}
		log.Error("failed to fetch user", "error", err, "user_id", userID)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to fetch user details",
		}
	}

	updateFields := []string{}
	args := []interface{}{userID}
	argCount := 1

	if dto.FirstName != nil {
		argCount++
		updateFields = append(updateFields, fmt.Sprintf("first_name = $%d", argCount))
		args = append(args, *dto.FirstName)
	}

	if dto.LastName != nil {
		argCount++
		updateFields = append(updateFields, fmt.Sprintf("last_name = $%d", argCount))
		args = append(args, *dto.LastName)
	}

	if dto.BusinessName != nil {
		argCount++
		updateFields = append(updateFields, fmt.Sprintf("business_name = NULLIF($%d, '')", argCount))
		args = append(args, *dto.BusinessName)
	}

	if dto.PhoneNumber != nil {
		argCount++
		updateFields = append(updateFields, fmt.Sprintf("phone_number = NULLIF($%d, '')", argCount))
		args = append(args, *dto.PhoneNumber)
	}

	if dto.Country != nil {
		argCount++
		updateFields = append(updateFields, fmt.Sprintf("country = NULLIF($%d, '')", argCount))
		args = append(args, *dto.Country)
	}

	if len(updateFields) == 0 {
		return &utils.ApiResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "No fields to update",
		}
	}

	updateFields = append(updateFields, "updated_at = NOW()")

	updateQuery := fmt.Sprintf(`
		UPDATE users 
		SET %s
		WHERE id = $1
		RETURNING 
			id, email, first_name, last_name, 
			business_name, phone_number, country, 
			email_verified, is_active, created_at, updated_at
	`, strings.Join(updateFields, ", "))

	var updatedProfile UserProfileDto
	err = tx.QueryRow(ctx, updateQuery, args...).Scan(
		&updatedProfile.ID,
		&updatedProfile.Email,
		&updatedProfile.FirstName,
		&updatedProfile.LastName,
		&updatedProfile.BusinessName,
		&updatedProfile.PhoneNumber,
		&updatedProfile.Country,
		&updatedProfile.EmailVerified,
		&updatedProfile.IsActive,
		&updatedProfile.CreatedAt,
		&updatedProfile.UpdatedAt,
	)

	if err != nil {
		log.Error("failed to update profile", "error", err, "user_id", userID)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to update profile",
		}
	}
	if err = tx.Commit(ctx); err != nil {
		log.Error("failed to commit transaction", "error", err)
		return &utils.ApiResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Failed to complete profile update",
		}
	}

	log.Info("user profile updated successfully", "user_id", userID)

	var address sql.NullString
	var usdc_balance sql.NullFloat64
	var xlm_balance sql.NullFloat64
	var walletId sql.NullString
	const q string = `
			SELECT id, address, usdc_balance, xlm_balance 
			FROM wallets 
			WHERE user_id = $1 AND is_primary = true AND is_active = true
		`
	err = s.postgres.QueryRow(ctx, q, userID).Scan(&walletId, &address, &usdc_balance, &xlm_balance)
	if err != nil {
		if err == pgx.ErrNoRows {
			s.log.Info("no wallet details found for user")
		}
	}

	wallet := &UserWallet{
		Balance: &WalletBalance{},
	}

	if address.Valid {
		wallet.Address = &address.String
	}
	if usdc_balance.Valid {
		wallet.Balance.USDC = &usdc_balance.Float64
	}
	if xlm_balance.Valid {
		wallet.Balance.XLM = &xlm_balance.Float64
	}

	if walletId.Valid {
		wallet.Id = &walletId.String
	}

	updatedProfile.Wallet = wallet

	return &utils.ApiResponse{
		StatusCode: http.StatusOK,
		Message:    "Profile updated successfully",
		Data:       updatedProfile,
	}
}
