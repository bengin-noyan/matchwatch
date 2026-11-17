include .env.example
-include .env

.PHONY: up down migrate run test test-integration

# Ortam değişkenleri yalnızca ilgili hedefe veriliyor; böylece düz `make test` entegrasyon testlerini atlar.
APP_ENV = DATABASE_URL='$(DATABASE_URL)' MIGRATE_DATABASE_URL='$(MIGRATE_DATABASE_URL)' JWT_SECRET='$(JWT_SECRET)' HTTP_ADDR='$(HTTP_ADDR)' STORAGE_DIR='$(STORAGE_DIR)'

up:
	docker compose up -d

down:
	docker compose down

migrate:
	cd backend-go && $(APP_ENV) go run ./cmd/migrate

run:
	cd backend-go && $(APP_ENV) go run ./cmd/api

test:
	cd backend-go && go test ./...

test-integration:
	cd backend-go && MATCHWATCH_TEST_DATABASE_URL='$(MATCHWATCH_TEST_DATABASE_URL)' MATCHWATCH_TEST_MIGRATE_URL='$(MATCHWATCH_TEST_MIGRATE_URL)' go test ./... -count=1
