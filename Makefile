# Continua Makefile
# ====================
# ONE generation command: `make generate`
# CI fails on drift after running generate

.PHONY: all setup generate build build-engine test test-go test-js lint lint-go lint-js \
        dev dev-server dev-web clean help migrate migrate-down docker docker-build \
        docker-up docker-down e2e

# Default target
all: generate build

# ============================================================================
# Setup
# ============================================================================

setup: ## Install all dependencies
	@echo "==> Installing Go dependencies..."
	go mod download
	cd engine && go mod download
	@echo "==> Installing Node.js dependencies..."
	pnpm install
	@echo "==> Installing Python SDK dependencies..."
	cd sdks/python && uv sync
	@echo "✅ Setup complete"

# ============================================================================
# Code Generation (ONE command to rule them all)
# ============================================================================

generate: ## Generate ALL code (contracts, sqlc, API types)
	@echo "==> Generating contracts..."
	pnpm --filter @continua/contracts generate
	@echo "==> Generating platform database code..."
	cd db/platform && sqlc generate
	@echo "==> Generating engine database code..."
	cd engine/db && sqlc generate
	@echo "==> Copying generated Go types..."
	@mkdir -p internal/api
	@if [ -f contracts/generated/go/server_gen.go ]; then \
		cp contracts/generated/go/server_gen.go internal/api/server_gen.go; \
	fi
	@echo "==> Generating Python SDK types..."
	@if [ -f contracts/openapi/openapi.bundle.yaml ]; then \
		cd sdks/python && uv run --with datamodel-code-generator python scripts/generate_types.py; \
	fi
	@echo "✅ All code generated"

# ============================================================================
# Build
# ============================================================================

build: build-server build-web ## Build everything

build-server: ## Build Go server binary
	@echo "==> Building continua server..."
	@mkdir -p bin
	go build -o bin/continua ./cmd/continua
	@echo "✅ Built bin/continua"

build-engine: ## Build engine binary
	@echo "==> Building continua-engine..."
	@mkdir -p bin
	cd engine && go build -o ../bin/continua-engine ./cmd/continua-engine
	@echo "✅ Built bin/continua-engine"

build-web: ## Build web UI
	@echo "==> Building web UI..."
	pnpm --filter web build
	@echo "==> Copying web assets to internal/web/static..."
	@rm -rf internal/web/static/*
	@cp -r web/dist/* internal/web/static/ 2>/dev/null || mkdir -p internal/web/static
	@touch internal/web/static/.gitkeep
	@echo "✅ Web UI built"

build-sdk: ## Build TypeScript SDK
	@echo "==> Building TypeScript SDK..."
	pnpm --filter @continua/sdk build
	@echo "✅ SDK built"

build-contracts: ## Build contracts package
	@echo "==> Building contracts..."
	pnpm --filter @continua/contracts build
	@echo "✅ Contracts built"

# ============================================================================
# Testing
# ============================================================================

test: test-go test-js ## Run all tests

test-go: ## Run Go tests
	@echo "==> Running Go tests..."
	go test -v -race ./...
	cd engine && go test -v -race ./...

test-js: ## Run JavaScript/TypeScript tests
	@echo "==> Running JS tests..."
	pnpm test

test-integration: ## Run integration tests (requires running database)
	@echo "==> Running integration tests..."
	go test -v -tags=integration ./...

test-all: test test-integration ## Run all tests including integration

e2e: ## Run E2E tests (requires running server and database)
	@echo "==> Running E2E tests..."
	cd sdks/python && uv run python examples/e2e_demo.py

# ============================================================================
# Linting
# ============================================================================

lint: lint-go lint-js ## Run all linters

lint-go: ## Run Go linters
	@echo "==> Linting Go code..."
	golangci-lint run ./...
	cd engine && golangci-lint run ./...

lint-js: ## Run JavaScript/TypeScript linters
	@echo "==> Linting JS/TS code..."
	pnpm lint

lint-fix: ## Fix linting issues
	@echo "==> Fixing Go lint issues..."
	golangci-lint run --fix ./...
	cd engine && golangci-lint run --fix ./...
	@echo "==> Fixing JS/TS lint issues..."
	pnpm lint:fix

type-check: ## Run TypeScript type checking
	pnpm type-check

# ============================================================================
# Development
# ============================================================================

dev: ## Start development database
	@echo "==> Starting development database..."
	docker compose -f deploy/docker-compose/docker-compose.dev.yml up -d
	@echo "✅ Database running on localhost:5432"

dev-server: ## Start Go server with hot reload
	@echo "==> Starting Go server with air..."
	$(HOME)/go/bin/air

dev-web: ## Start web UI development server
	@echo "==> Starting web UI dev server..."
	pnpm dev:web

dev-stop: ## Stop development services
	@echo "==> Stopping development services..."
	docker compose -f deploy/docker-compose/docker-compose.dev.yml down

# ============================================================================
# Database Migrations
# ============================================================================

migrate: ## Run database migrations
	@echo "==> Running migrations..."
	go run ./cmd/continua migrate up

migrate-down: ## Rollback last migration
	@echo "==> Rolling back migration..."
	go run ./cmd/continua migrate down 1

migrate-create: ## Create a new migration (usage: make migrate-create name=add_users)
	@echo "==> Creating migration..."
	migrate create -ext sql -dir db/platform/migrations/postgres -seq $(name)

# ============================================================================
# Docker
# ============================================================================

docker: docker-build docker-up ## Build and start Docker containers

docker-build: ## Build Docker image
	@echo "==> Building Docker image..."
	docker build -f deploy/docker/Dockerfile -t continua:latest .

docker-up: ## Start Docker containers
	@echo "==> Starting Docker containers..."
	docker compose -f deploy/docker-compose/docker-compose.yml up -d

docker-down: ## Stop Docker containers
	@echo "==> Stopping Docker containers..."
	docker compose -f deploy/docker-compose/docker-compose.yml down

docker-logs: ## View Docker logs
	docker compose -f deploy/docker-compose/docker-compose.yml logs -f

# ============================================================================
# Cleanup
# ============================================================================

clean: ## Clean build artifacts
	@echo "==> Cleaning..."
	rm -rf bin/ tmp/ coverage.out coverage.html
	rm -rf web/dist/
	rm -rf internal/web/static/*
	touch internal/web/static/.gitkeep
	pnpm clean
	@echo "✅ Cleaned"

clean-all: clean ## Clean everything including dependencies
	rm -rf node_modules/
	rm -rf contracts/node_modules/
	rm -rf web/node_modules/
	rm -rf sdks/typescript/node_modules/
	rm -rf sdks/python/.venv/

# ============================================================================
# CI
# ============================================================================

ci-check: ## Check that generated code is in sync
	@echo "==> Checking generated code..."
	@./scripts/check-generated.sh

ci: generate lint test ## Run full CI pipeline locally

# ============================================================================
# Help
# ============================================================================

help: ## Show this help
	@echo "Continua Makefile"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'
