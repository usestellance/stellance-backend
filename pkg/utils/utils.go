package utils

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
	"unicode"

	"github.com/The-True-Hooha/stellance-backend.git/internal/middleware"
	"github.com/fernet/fernet-go"
	"github.com/go-playground/validator/v10"
	gonanoid "github.com/matoous/go-nanoid/v2"
	"golang.org/x/crypto/bcrypt"
)

var (
	emailKey *fernet.Key
	keyOnce  sync.Once
	keyErr   error
)

type CurrencyType string
type OrderByType string

const (
	USDC        CurrencyType = "usdc"
	XLM         CurrencyType = "xlm"
	OrderByDESC OrderByType  = "DESC"
	OrderByASC  OrderByType  = "ASC"
)

type ApiResponse struct {
	StatusCode int    `json:"status_code"`
	Message    string `json:"message"`
	Data       any    `json:"data,omitempty"`
	Error      string `json:"error,omitempty"`
}

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ErrorResponse struct {
	Status  int               `json:"status"`
	Message string            `json:"message"`
	Errors  []ValidationError `json:"errors,omitempty"`
}

func GetEnvAsInt() int {
	if valueStr := os.Getenv("PG_PORT"); valueStr != "" {
		if value, err := strconv.Atoi(valueStr); err == nil {
			return value
		}
	}
	return 5433
}

func HashString(data string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(data), bcrypt.DefaultCost)
	return string(hash), err
}

func CompareHash(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

func HandleValidationError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)

	res := ErrorResponse{
		Status:  http.StatusBadRequest,
		Message: "failed! invalid request details sent",
		Errors:  make([]ValidationError, 0),
	}
	switch ve := err.(type) {
	case validator.ValidationErrors:
		seen := make(map[string]bool)
		for _, e := range ve {
			field := e.Field()
			if !seen[field] {
				res.Errors = append(res.Errors, ValidationError{
					Field:   field,
					Message: parseValidationErrorMessage(e),
				})
				seen[field] = true
			}
		}
	default:
		res.Errors = append(res.Errors, ValidationError{
			Field:   "unknown",
			Message: err.Error(),
		})
	}

	_ = json.NewEncoder(w).Encode(res)
}

func parseValidationErrorMessage(e validator.FieldError) string {
	switch e.Tag() {
	case "required":
		return fmt.Sprintf("%s is required", e.Field())
	case "email":
		return fmt.Sprintf("%s must be a valid email address", e.Field())
	case "min":
		return fmt.Sprintf("%s must be at least %s characters long", e.Field(), e.Param())
	case "max":
		return fmt.Sprintf("%s must be at most %s characters long", e.Field(), e.Param())
	case "len":
		return fmt.Sprintf("%s must be exactly %s characters long", e.Field(), e.Param())
	default:
		return fmt.Sprintf("%s is invalid", e.Field())
	}
}

func WriteToJson(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	json.NewEncoder(w).Encode(data)
}

func EncryptEmail(email string) (string, error) {
	keyOnce.Do(initEmailKey)
	if keyErr != nil {
		return "", fmt.Errorf("encryption key error: %w", keyErr)
	}
	token, err := fernet.EncryptAndSign([]byte(email), emailKey)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt email: %w", err)
	}
	return base64.URLEncoding.EncodeToString(token), nil
}

func DecryptEmail(token string) (string, error) {
	keyOnce.Do(initEmailKey)
	if keyErr != nil {
		return "", fmt.Errorf("encryption key error: %w", keyErr)
	}
	tokenBytes, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return "", fmt.Errorf("invalid token format: %w", err)
	}
	decoded := fernet.VerifyAndDecrypt(tokenBytes, 7*24*time.Hour, []*fernet.Key{emailKey})
	if decoded == nil {
		return "", fmt.Errorf("invalid or expired email token")
	}
	return string(decoded), nil
}

func initEmailKey() {
	keyString := os.Getenv("EMAIL_KEY")
	if keyString == "" {
		keyErr = fmt.Errorf("EMAIL_ENCRYPTION_KEY environment variable not set")
		return
	}

	emailKey, keyErr = fernet.DecodeKey(keyString)
}

func GetUserIDFromContext(ctx context.Context) (string, bool) {
	userId, ok := ctx.Value(middleware.UserIDKey).(string)
	return userId, ok
}

func GetUserEmailFromContext(ctx context.Context) (string, bool) {
	email, ok := ctx.Value(middleware.UserEmailKey).(string)
	return email, ok
}

func GetRoleFromContext(ctx context.Context) (string, bool) {
	role, ok := ctx.Value(middleware.RoleKey).(string)
	return role, ok
}

func GetBaseURL() string {
	return os.Getenv("BASE_URL")
}

func ValidatePassword(fl validator.FieldLevel) bool {
	password := fl.Field().String()

	if len(password) < 8 {
		return false
	}

	var (
		hasUpper   bool
		hasLower   bool
		hasNumber  bool
		hasSpecial bool
	)

	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsNumber(char):
			hasNumber = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	return hasUpper && hasLower && hasNumber && hasSpecial
}

func CheckPasswordRequirements(password string) (bool, string) {
	if len(password) < 8 {
		return false, "Password must be at least 8 characters long"
	}
	var (
		hasUpper   bool
		hasLower   bool
		hasNumber  bool
		hasSpecial bool
	)

	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsNumber(char):
			hasNumber = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	if !hasUpper {
		return false, "Password must contain at least one uppercase letter"
	}
	if !hasLower {
		return false, "Password must contain at least one lowercase letter"
	}
	if !hasNumber {
		return false, "Password must contain at least one number"
	}
	if !hasSpecial {
		return false, "Password must contain at least one special character"
	}

	return true, ""
}

func GenerateShortURL(data string, log *slog.Logger) (string, error) {
	shortID, err := gonanoid.Generate(data, 30)
	if err != nil {
		log.Warn("failed to generate url from nano id returning to default")
		b := make([]byte, 30)
		for i := range b {
			n, err := rand.Int(rand.Reader, big.NewInt(int64(len(data))))
			if err != nil {
				return "", err
			}
			b[i] = data[n.Int64()]
		}
		return string(b), nil
	}
	log.Debug("generate url from go nano id")
	return shortID, nil
}

func GenerateSecureToken(length int) (string, error) {
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func GetStellarStage() string {
	if os.Getenv("STELLAR_STAGE") == "dev" {
		return "testnet"
	}
	return "mainnet"
}

func NullStringPtr(ns sql.NullString) *string {
	if ns.Valid {
		return &ns.String
	}
	return nil
}

func NullFloatPtr(nf sql.NullFloat64) *float64 {
	if nf.Valid {
		return &nf.Float64
	}
	return nil
}
