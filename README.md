# Stellance Backend

A modern payment and invoice management API built with Go, PostgreSQL, and Redis. Stellance enables businesses to create, manage, and track invoices with integrated Stellar blockchain payments.

## 📋 Prerequisites

- Go 1.22 or higher
- PostgreSQL 16+
- Redis 7+
- Docker & Docker Compose (optional but recommended)

## Server Configuration

STAGE=dev
BASE_URL=<http://localhost:4000>

- Postgres
DB_HOST=localhost
DB_PORT=5432
DB_USER=stellance
DB_PASSWORD=stellance_dev_password
DB_NAME=stellance_db
DB_SSL_MODE=disable if stage == dev : require

- Redis
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=your password
REDIS_DB=0

- JWT
JWT_SECRET=your-super-secret-jwt-key-at-least-32-chars
JWT_REFRESH_SECRET=different-secret-for-refresh-tokens

- Email
EMAIL_ENCRYPTION_KEY=your-fernet-key-here
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USER=<your-email@gmail.com>
SMTP_PASSWORD=your-app-password

- Cors
CORS_ALLOWED_ORIGINS=<http://localhost:3000,http://localhost:5173>

SERVICE_FEE_PERCENTAGE=2.5

## Migrations

Run `migrations` with with [go-migrate](https://github.com/golang-migrate/migrate)

## 🏃‍♂️ Running the Application

- Development Mode

Run with hot reload (install air first)

cmd: `go install github.com/cosmtrek/air@latest`

cmd: `air`

- Or run directly

`go run cmd/api/main.go`
