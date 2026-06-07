.PHONY: build run test lint sqlc migrate-up migrate-down tidy

SERVICE_NAME := user-service
BINARY := bin/$(SERVICE_NAME)

build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BINARY) ./cmd/user

run:
	go run ./cmd/user

test:
	go test -race -cover ./...

lint:
	golangci-lint run ./...

sqlc:
	sqlc generate

migrate-create:
	@if [ -z "$(name)" ]; then echo "Usage: make migrate-create name=<migration_name>"; exit 1; fi
	goose -dir db/migrations create $(name) sql

migrate-up:
	goose -dir db/migrations postgres "$(DATABASE_URL)" up

migrate-down:
	goose -dir db/migrations postgres "$(DATABASE_URL)" down

tidy:
	go mod tidy