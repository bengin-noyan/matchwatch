include .env.example
export

.PHONY: up down migrate run test test-integration

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
