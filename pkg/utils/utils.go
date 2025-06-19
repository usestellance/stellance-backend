package utils

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/fernet/fernet-go"
	"github.com/go-playground/validator/v10"
	"golang.org/x/crypto/bcrypt"
)

var (
	emailKey *fernet.Key
	keyOnce  sync.Once
	keyErr   error
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
	}

	if ve, ok := err.(validator.ValidationErrors); ok {
		for _, err := range ve {
			res.Errors = append(res.Errors, ValidationError{

				Field:   err.Field(),
				Message: parseValidationErrorMessage(err),
			})
		}
	} else {
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
		return "is required"
	case "email":
		return "must be a valid email"
	case "min":
		return "must be at least " + e.Param() + " characters"
	case "max":
		return "must be at most " + e.Param() + " characters"
	case "len":
		return "must be exactly " + e.Param() + " characters"
	default:
		return "is invalid"
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
