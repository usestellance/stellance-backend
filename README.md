# Stellance Backend

A modern payment and invoice management API built with Go, PostgreSQL, and Redis. Stellance enables businesses to create, manage, and track invoices with integrated Stellar blockchain payments.

## 📋 Prerequisites

- Go 1.22 or higher
- PostgreSQL 16+
- Redis 7+
- Docker & Docker Compose (optional but recommended)

## Server Configuration

!! Check .env.example

## Migrations

Run `migrations` with with [go-migrate](https://github.com/golang-migrate/migrate)

## 🏃‍♂️ Running the Application

- Development Mode

Run with hot reload (install air first)

cmd: `go install github.com/air-verse/air@latest`

cmd: `air`

- Or run directly

`go run cmd/api/main.go`

`//go:embed templates/**`
