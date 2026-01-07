DB_HOST ?=localhost
DB_PORT ?=5432
DB_USER ?=postgres
DB_PASSWORD ?=davidhero125.
DB_NAME ?= stellance
DB_SSL ?=disable

DATABASE_URL ?= postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=$(DB_SSL)
MIGRATIONS_PATH ?= migrations

.PHONY: migrate-up migrate-down migrate-create migrate-version migrate-force

migrate-up:
	migrate -path $(MIGRATIONS_PATH) -database "$(DATABASE_URL)" up

migrate-down:
	migrate -path $(MIGRATIONS_PATH) -database "$(DATABASE_URL)" down 1

migrate-create:
	@read -p "Migration name: " name; \
	migrate create -ext sql -dir $(MIGRATIONS_PATH) -seq $$name

migrate-version:
	migrate -path $(MIGRATIONS_PATH) -database "$(DATABASE_URL)" version

migrate-force:
	migrate -path $(MIGRATIONS_PATH) -database "$(DATABASE_URL)" force $(version)

migrate-goto:
	@read -p "Target version: " version; \
	migrate -path $(MIGRATIONS_PATH) -database "$(DATABASE_URL)" goto $$version