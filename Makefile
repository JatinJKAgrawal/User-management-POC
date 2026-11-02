SHELL := /bin/bash
include $(wildcard .env)
export

.PHONY: help run build test lint fmt migrate-up migrate-down migrate-version migrate-create \
        docker-up docker-down docker-logs docker-build smoke

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

run: ## Run the API locally (needs Postgres; see .env)
	go run ./cmd/api

build: ## Build binaries into ./bin
	CGO_ENABLED=0 go build -trimpath -o bin/api ./cmd/api
	CGO_ENABLED=0 go build -trimpath -o bin/migrate ./cmd/migrate

test: ## Run unit tests
	go test -race ./...

lint: ## Run go vet and check formatting
	go vet ./...
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed:"; gofmt -l .; exit 1)

fmt: ## Format code
	gofmt -w .

migrate-up: ## Apply all migrations
	go run ./cmd/migrate up

migrate-down: ## Roll back the last migration
	go run ./cmd/migrate down 1

migrate-version: ## Show current migration version
	go run ./cmd/migrate version

migrate-create: ## Create a new migration pair: make migrate-create name=add_x
	@test -n "$(name)" || (echo "usage: make migrate-create name=<name>"; exit 1)
	@n=$$(printf "%06d" $$(( $$(ls migrations/*.up.sql | wc -l) + 1 ))); \
	touch migrations/$${n}_$(name).up.sql migrations/$${n}_$(name).down.sql; \
	echo "created migrations/$${n}_$(name).{up,down}.sql"

docker-up: ## Start postgres, mailhog, migrations and the API
	docker compose up -d --build

docker-down: ## Stop all containers
	docker compose down

docker-logs: ## Tail API logs
	docker compose logs -f api

docker-build: ## Build the API image
	docker build -t user-management-api:local .

smoke: ## Run the end-to-end smoke test against a running stack
	./scripts/smoke_test.sh
