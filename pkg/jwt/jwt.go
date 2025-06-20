package jwt_

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/The-True-Hooha/stellance-backend.git/pkg/config"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type JwtTokenServiceConfig struct {
	log                *slog.Logger
	postgres           *pgxpool.Pool
	redis              *redis.Client
	secret             string
	refreshTokenSecret string
}

type Claims struct {
	UserId string `json:"user_id"`
	Email  string `json:"email"`
	Role string `json:"role"`
	jwt.RegisteredClaims
}

func JwtTokenService() *JwtTokenServiceConfig {
	return &JwtTokenServiceConfig{
		log:                config.GetAppContainer().Log,
		postgres:           config.GetAppContainer().Postgres,
		redis:              config.GetAppContainer().Redis,
		secret:             os.Getenv("JWT_SECRET"),
		refreshTokenSecret: os.Getenv("REFRESH_TOKEN_SECRET"),
	}
}

func (config *JwtTokenServiceConfig) GenerateNewAccessToken(userId, email, role string) (string, error) {
	claims := Claims{
		UserId: userId,
		Email:  email,
		Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "stellance",
			Audience:  []string{"stellance:web"},
			Subject:   userId,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.secret))
}

func (config *JwtTokenServiceConfig) ValidateAccessToken(accessToken string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(accessToken, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(config.secret), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}
	return nil, fmt.Errorf("failed! invalid token")
}

func (config *JwtTokenServiceConfig) GenerateRefreshToken(accessToken string) (string, error) {
	claims, err := config.ValidateAccessToken(accessToken)
	if err != nil {
		return "", fmt.Errorf("invalid access token: %w", err)
	}
	refreshClaims := Claims{
		UserId: claims.UserId,
		Email:  claims.Email,
		Role: claims.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "stellance",
			Subject:   claims.UserId,
			Audience:  []string{"stellance:web"},
		},
	}
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	tokenString, err := refreshToken.SignedString([]byte(config.refreshTokenSecret))
	if err != nil {
		return "", fmt.Errorf("failed to sign refresh token: %w", err)
	}

	return tokenString, nil
}
