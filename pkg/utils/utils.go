package utils

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/The-True-Hooha/stellance-backend/internal/middleware"
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
	Status  int    `json:"status"`
	Message string `json:"message"`
}

type InvoiceAccessData struct {
	InvoiceURL string `json:"url"`
	Email      string `json:"email"`
	Name       string `json:"name"`
}

func GetEnvAsInt() int {
	if valueStr := os.Getenv("PG_PORT"); valueStr != "" {
		if value, err := strconv.Atoi(valueStr); err == nil {
			return value
		}
	}
	return 5433
}

func GenerateInvoiceAccessToken(invoiceURL, email, name, secret string) (string, error) {
	payload := InvoiceAccessData{
		InvoiceURL: invoiceURL,
		Email:      strings.ToLower(strings.TrimSpace(email)),
		Name:       name,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, jsonData, nil)
	token := base64.URLEncoding.EncodeToString(ciphertext)
	return token, nil
}

func DecryptInvoiceAccessToken(token, secret string) (*InvoiceAccessData, error) {
	ciphertext, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("invalid token format: %w", err)
	}

	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("invalid token: too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid token: decryption failed")
	}

	var payload InvoiceAccessData
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return nil, fmt.Errorf("invalid token data: %w", err)
	}

	return &payload, nil
}

func VerifyInvoiceAccessToken(invoiceURL, token, secret string) (*InvoiceAccessData, error) {
	payload, err := DecryptInvoiceAccessToken(token, secret)
	if err != nil {
		return nil, err
	}

	if payload.InvoiceURL != invoiceURL {
		return nil, fmt.Errorf("token not valid for this invoice")
	}

	return payload, nil
}

func HashString(data string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(data), bcrypt.DefaultCost)
	return string(hash), err
}

func humanizeFieldName(name string) string {
	var words []rune
	for i, r := range name {
		if i > 0 && r >= 'A' && r <= 'Z' {
			words = append(words, ' ')
		}
		words = append(words, r)
	}
	return string(words)
}

func CompareHash(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

func HandleValidationError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)

	message := "failed! invalid request details sent"

	switch ve := err.(type) {
	case validator.ValidationErrors:
		seen := make(map[string]bool)
		var sb strings.Builder
		for _, e := range ve {
			field := e.Field()
			if !seen[field] {
				sb.WriteString(fmt.Sprintf("%s; ", parseValidationErrorMessage(e)))
				seen[field] = true
			}
		}
		message = sb.String()

	default:
		message = err.Error()
	}

	res := ErrorResponse{
		Status:  http.StatusBadRequest,
		Message: message,
	}

	_ = json.NewEncoder(w).Encode(res)
}

func parseValidationErrorMessage(e validator.FieldError) string {
	field := humanizeFieldName(e.Field())
	param := e.Param()

	switch e.Tag() {
	case "required":
		return fmt.Sprintf("%s is required", field)

	case "email":
		return fmt.Sprintf("%s must be a valid email address", field)

	case "min":
		return fmt.Sprintf("%s must be at least %s characters long", field, param)

	case "max":
		return fmt.Sprintf("%s must be at most %s characters long", field, param)

	case "len":
		return fmt.Sprintf("%s must be exactly %s characters long", field, param)

	case "eq":
		return fmt.Sprintf("%s must be equal to %s", field, param)

	case "ne":
		return fmt.Sprintf("%s must not be equal to %s", field, param)

	case "gte":
		return fmt.Sprintf("%s must be greater than or equal to %s", field, param)

	case "lte":
		return fmt.Sprintf("%s must be less than or equal to %s", field, param)

	case "oneof":
		return fmt.Sprintf("%s must be one of [%s]", field, param)

	case "url":
		return fmt.Sprintf("%s must be a valid URL", field)

	case "uuid":
		return fmt.Sprintf("%s must be a valid UUID", field)

	case "numeric":
		return fmt.Sprintf("%s must be a numeric value", field)

	case "boolean":
		return fmt.Sprintf("%s must be a boolean", field)

	case "datetime":
		return fmt.Sprintf("%s must be a valid datetime format", field)
	case "passwd":
		return string("password length must be greater than 6 and contain one upper case, lower case, numeric, and a special characters")

	default:
		return fmt.Sprintf("%s is invalid (%s validation failed)", field, e.Tag())
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

	if len(password) < 6 {
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
	if len(password) < 6 {
		return false, "Password must be at least 6 characters long"
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

func ConvertUSDCToCents(usdcAmount string) (int64, error) {
	amount, err := strconv.ParseFloat(usdcAmount, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse USDC amount: %w", err)
	}

	cents := int64(math.Round(amount * 100))
	return cents, nil
}

func ParseMonthString(monthStr string) (time.Month, error) {
	if monthStr == "" {
		return 0, fmt.Errorf("empty month string")
	}

	monthStr = strings.ToLower(strings.TrimSpace(monthStr))

	monthMap := map[string]time.Month{
		"january": time.January, "jan": time.January,
		"february": time.February, "feb": time.February,
		"march": time.March, "mar": time.March,
		"april": time.April, "apr": time.April,
		"may":  time.May,
		"june": time.June, "jun": time.June,
		"july": time.July, "jul": time.July,
		"august": time.August, "aug": time.August,
		"september": time.September, "sep": time.September, "sept": time.September,
		"october": time.October, "oct": time.October,
		"november": time.November, "nov": time.November,
		"december": time.December, "dec": time.December,
	}

	if month, exists := monthMap[monthStr]; exists {
		return month, nil
	}

	monthNum, err := strconv.Atoi(monthStr)
	if err == nil && monthNum >= 1 && monthNum <= 12 {
		return time.Month(monthNum), nil
	}

	return 0, fmt.Errorf("invalid month format: %s", monthStr)
}

func CapitalizeStatus(status string) string {
	if status == "" {
		return status
	}
	return strings.ToUpper(status[:1]) + strings.ToLower(status[1:])
}