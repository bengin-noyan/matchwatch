include .env.example
-include .env

.PHONY: up down migrate run test test-integration

# Env is exported per target only, so plain `make test` skips integration tests.
migrate run: export DATABASE_URL MIGRATE_DATABASE_URL JWT_SECRET HTTP_ADDR STORAGE_DIR
test-integration: export MATCHWATCH_TEST_DATABASE_URL MATCHWATCH_TEST_MIGRATE_URL

up:
	docker compose up -d

down:
	docker compose down

migrate:
	cd backend-go && go run ./cmd/migrate

run:
	cd backend-go && go run ./cmd/api

test:
	cd backend-go && go test ./...

test-integration:
	cd backend-go && go test ./... -count=1
