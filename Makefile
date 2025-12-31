# ═══════════════════════════════════════════════════════════════════
# CONTINUA MONOREPO MAKEFILE
# ═══════════════════════════════════════════════════════════════════

# Shell configuration (required for trap to work reliably)
SHELL := bash
.ONESHELL:
.SHELLFLAGS := -euo pipefail -c

# ═══════════════════════════════════════════════════════════════════
# VARIABLES
# ═══════════════════════════════════════════════════════════════════

CYAN := \033[36m
GREEN := \033[32m
YELLOW := \033[33m
RED := \033[31m
RESET := \033[0m

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

LDFLAGS := -ldflags "\
    -X github.com/aryanVijaywargia/Continua/server/internal/version.Version=$(VERSION) \
    -X github.com/aryanVijaywargia/Continua/server/internal/version.Commit=$(COMMIT) \
    -X github.com/aryanVijaywargia/Continua/server/internal/version.BuildTime=$(BUILD_TIME)"

MIGRATE_VERSION := v4.17.0
MIGRATE_IMAGE := migrate/migrate:$(MIGRATE_VERSION)

COMPOSE_DEV := docker compose -f docker/docker-compose.yml
COMPOSE_TEST := docker compose -f docker/docker-compose.test.yml

GENERATED_PATHS := \
    packages/proto-go/continua \
    packages/api-client/src/generated \
    sdks/python/src/continua/_generated

# ═══════════════════════════════════════════════════════════════════
# DEFAULT TARGET
# ═══════════════════════════════════════════════════════════════════

.PHONY: all
all: proto server sdk-python sdk-ts web
	@echo "$(GREEN)✓ Full build complete$(RESET)"

.PHONY: help
help:
	@echo "$(CYAN)Continua Build System$(RESET)"
	@echo ""
	@echo "$(YELLOW)Setup:$(RESET)"
	@echo "  make setup          First-time development setup"
	@echo "  make doctor         Check development environment"
	@echo ""
	@echo "$(YELLOW)Build:$(RESET)"
	@echo "  make all            Build everything"
	@echo "  make proto          Generate protobuf code (all languages)"
	@echo "  make server         Build Go server"
	@echo "  make sdk-python     Build Python SDK"
	@echo "  make sdk-ts         Build TypeScript SDK"
	@echo "  make web            Build web UI"
	@echo ""
	@echo "$(YELLOW)Development:$(RESET)"
	@echo "  make dev            Start all services for development"
	@echo "  make server-run     Run server only"
	@echo "  make web-dev        Run web UI in dev mode"
	@echo ""
	@echo "$(YELLOW)Testing:$(RESET)"
	@echo "  make test           Run all unit tests"
	@echo "  make test-integration  Run integration tests"
	@echo "  make test-all       Run all tests"
	@echo ""
	@echo "$(YELLOW)Quality:$(RESET)"
	@echo "  make lint           Run all linters"
	@echo "  make proto-lint     Lint proto files"
	@echo "  make ci-check       Run all CI checks"
	@echo ""
	@echo "$(YELLOW)Database:$(RESET)"
	@echo "  make migrate-up     Run migrations"
	@echo "  make migrate-down   Rollback one migration"
	@echo "  make migrate-create Create new migration"
	@echo ""
	@echo "$(YELLOW)Docker:$(RESET)"
	@echo "  make docker-build   Build Docker images"
	@echo "  make docker-up      Start Docker services"
	@echo "  make docker-down    Stop Docker services"
	@echo ""
	@echo "$(YELLOW)Other:$(RESET)"
	@echo "  make clean          Clean all build artifacts"

# ═══════════════════════════════════════════════════════════════════
# SETUP & DOCTOR
# ═══════════════════════════════════════════════════════════════════

.PHONY: setup
setup:
	@echo "$(CYAN)Setting up development environment...$(RESET)"
	@./scripts/setup-dev.sh

.PHONY: doctor
doctor:
	@echo "$(CYAN)Checking development environment...$(RESET)"
	@echo ""
	@echo "Required tools:"
	@printf "  go:      " && (go version 2>/dev/null | cut -d' ' -f3 || echo "$(RED)NOT FOUND$(RESET) - Install: https://go.dev/dl/")
	@printf "  node:    " && (node --version 2>/dev/null || echo "$(RED)NOT FOUND$(RESET) - Install: https://nodejs.org/")
	@printf "  pnpm:    " && (pnpm --version 2>/dev/null || echo "$(RED)NOT FOUND$(RESET) - Run: corepack enable")
	@printf "  python:  " && (python3 --version 2>/dev/null | cut -d' ' -f2 || echo "$(RED)NOT FOUND$(RESET)")
	@printf "  uv:      " && (uv --version 2>/dev/null | cut -d' ' -f2 || echo "$(RED)NOT FOUND$(RESET) - Install: curl -LsSf https://astral.sh/uv/install.sh | sh")
	@printf "  buf:     " && (buf --version 2>/dev/null || echo "$(RED)NOT FOUND$(RESET) - Install: https://buf.build/docs/installation")
	@printf "  docker:  " && (docker --version 2>/dev/null | cut -d' ' -f3 | tr -d ',' || echo "$(RED)NOT FOUND$(RESET)")
	@echo ""
	@echo "Optional tools:"
	@printf "  mise:    " && (mise --version 2>/dev/null || echo "not installed")
	@printf "  golangci-lint: " && (golangci-lint --version 2>/dev/null | head -1 | cut -d' ' -f4 || echo "not installed")
	@echo ""
	@echo "$(GREEN)Run 'make setup' to install dependencies.$(RESET)"

# ═══════════════════════════════════════════════════════════════════
# PROTO GENERATION
# ═══════════════════════════════════════════════════════════════════

.PHONY: proto
proto:
	@echo "$(CYAN)Generating protobuf code...$(RESET)"
	cd proto && buf generate
	@echo "$(GREEN)✓ Proto generation complete$(RESET)"

.PHONY: proto-fmt
proto-fmt:
	@echo "$(CYAN)Formatting proto files...$(RESET)"
	cd proto && buf format -w
	@echo "$(GREEN)✓ Proto files formatted$(RESET)"

.PHONY: proto-fmt-check
proto-fmt-check:
	@echo "$(CYAN)Checking proto formatting...$(RESET)"
	cd proto && buf format -d --exit-code || (echo "$(RED)Proto files not formatted. Run 'make proto-fmt'$(RESET)" && exit 1)
	@echo "$(GREEN)✓ Proto files are formatted$(RESET)"

.PHONY: proto-lint
proto-lint: proto-fmt-check
	@echo "$(CYAN)Linting proto files...$(RESET)"
	cd proto && buf lint
	@echo "$(GREEN)✓ Proto lint passed$(RESET)"

.PHONY: proto-breaking
proto-breaking:
	cd proto && buf breaking --against '.git#branch=main'

.PHONY: proto-check
proto-check: proto-fmt-check
	@echo "$(CYAN)Checking generated code is up-to-date...$(RESET)"
	cd proto && buf generate
	@if git diff --name-only -- $(GENERATED_PATHS) | grep -q .; then \
		echo "$(RED)Generated code is out of date. Run 'make proto' and commit:$(RESET)"; \
		git diff --name-only -- $(GENERATED_PATHS); \
		exit 1; \
	fi
	@echo "$(GREEN)✓ Generated code is up-to-date$(RESET)"

.PHONY: proto-clean
proto-clean:
	rm -rf packages/proto-go/continua
	rm -rf packages/api-client/src/generated/continua
	rm -rf sdks/python/src/continua/_generated/continua

# ═══════════════════════════════════════════════════════════════════
# GO WORKSPACE
# ═══════════════════════════════════════════════════════════════════

.PHONY: go-work-check
go-work-check:
	@echo "$(CYAN)Checking go.work is in sync...$(RESET)"
	cp go.work go.work.bak
	cp go.work.sum go.work.sum.bak 2>/dev/null || touch go.work.sum.bak
	go work sync
	if ! diff -q go.work go.work.bak > /dev/null 2>&1 || ! diff -q go.work.sum go.work.sum.bak > /dev/null 2>&1; then \
		echo "$(RED)go.work or go.work.sum is out of sync. Run 'go work sync' and commit.$(RESET)"; \
		mv go.work.bak go.work; \
		mv go.work.sum.bak go.work.sum 2>/dev/null || true; \
		exit 1; \
	fi
	rm go.work.bak go.work.sum.bak 2>/dev/null || true
	@echo "$(GREEN)✓ go.work is in sync$(RESET)"

# ═══════════════════════════════════════════════════════════════════
# SERVER (Go)
# ═══════════════════════════════════════════════════════════════════

.PHONY: server
server:
	@echo "$(CYAN)Building server...$(RESET)"
	@echo "  Version: $(VERSION)"
	@echo "  Commit:  $(COMMIT)"
	cd server && go build $(LDFLAGS) -o ../bin/continua-server ./cmd/continua-server
	cd server && go build $(LDFLAGS) -o ../bin/continua-admin ./cmd/continua-admin
	cd server && go build -o ../bin/healthcheck ./cmd/healthcheck
	@echo "$(GREEN)✓ Server build complete$(RESET)"

.PHONY: server-run
server-run:
	cd server && go run $(LDFLAGS) ./cmd/continua-server

.PHONY: server-test
server-test:
	cd server && go test -race -cover ./...

.PHONY: server-lint
server-lint:
	cd server && golangci-lint run ./...

.PHONY: server-tidy
server-tidy:
	cd server && go mod tidy
	cd packages/proto-go && go mod tidy

# ═══════════════════════════════════════════════════════════════════
# PYTHON SDK
# ═══════════════════════════════════════════════════════════════════

.PHONY: sdk-python
sdk-python:
	@echo "$(CYAN)Building Python SDK...$(RESET)"
	cd sdks/python && uv sync
	@echo "$(GREEN)✓ Python SDK ready$(RESET)"

.PHONY: sdk-python-test
sdk-python-test:
	cd sdks/python && uv run pytest -v

.PHONY: sdk-python-lint
sdk-python-lint:
	cd sdks/python && uv run ruff check .
	cd sdks/python && uv run mypy src/

.PHONY: sdk-python-fmt
sdk-python-fmt:
	cd sdks/python && uv run ruff format .

# ═══════════════════════════════════════════════════════════════════
# TYPESCRIPT (SDK + API Client + Web)
# ═══════════════════════════════════════════════════════════════════

.PHONY: api-client
api-client:
	@echo "$(CYAN)Building API client...$(RESET)"
	pnpm --filter @continua/api-client build
	@echo "$(GREEN)✓ API client build complete$(RESET)"

.PHONY: sdk-ts
sdk-ts: api-client
	@echo "$(CYAN)Building TypeScript SDK...$(RESET)"
	pnpm --filter @continua/sdk build
	@echo "$(GREEN)✓ TypeScript SDK build complete$(RESET)"

.PHONY: web
web: api-client
	@echo "$(CYAN)Building web UI...$(RESET)"
	pnpm --filter @continua/web build
	@echo "$(GREEN)✓ Web UI build complete$(RESET)"

.PHONY: web-dev
web-dev:
	pnpm --filter @continua/web dev

.PHONY: ts-test
ts-test:
	pnpm test

.PHONY: ts-lint
ts-lint:
	pnpm lint

# ═══════════════════════════════════════════════════════════════════
# DATABASE MIGRATIONS
# ═══════════════════════════════════════════════════════════════════

.PHONY: migrate-up
migrate-up:
	$(COMPOSE_DEV) run --rm migrate up

.PHONY: migrate-down
migrate-down:
	$(COMPOSE_DEV) run --rm migrate down 1

.PHONY: migrate-drop
migrate-drop:
	$(COMPOSE_DEV) run --rm migrate drop -f

.PHONY: migrate-version
migrate-version:
	$(COMPOSE_DEV) run --rm migrate version

.PHONY: migrate-force
migrate-force:
	@read -p "Force version: " version; \
	$(COMPOSE_DEV) run --rm migrate force $$version

.PHONY: migrate-create
migrate-create:
	@read -p "Migration name: " name; \
	docker run --rm -v $(PWD)/server/schema/migrations:/migrations \
		$(MIGRATE_IMAGE) \
		create -ext sql -dir /migrations -seq $$name

# ═══════════════════════════════════════════════════════════════════
# TESTING
# ═══════════════════════════════════════════════════════════════════

.PHONY: test
test: server-test sdk-python-test ts-test
	@echo "$(GREEN)✓ All unit tests passed$(RESET)"

.PHONY: test-integration
test-integration:
	@echo "$(CYAN)Running Integration Tests...$(RESET)"
	cleanup() {
		echo "$(CYAN)Cleaning up...$(RESET)"
		$(COMPOSE_TEST) down -v --remove-orphans 2>/dev/null || true
	}
	trap cleanup EXIT
	$(COMPOSE_TEST) up -d --build --wait --wait-timeout 120 db server
	echo "$(CYAN)Running Python tests...$(RESET)"
	$(COMPOSE_TEST) run --rm tests-python
	echo "$(CYAN)Running TypeScript tests...$(RESET)"
	$(COMPOSE_TEST) run --rm tests-typescript
	echo "$(GREEN)✓ All integration tests passed$(RESET)"

.PHONY: test-integration-python
test-integration-python:
	cleanup() { $(COMPOSE_TEST) down -v --remove-orphans 2>/dev/null || true; }
	trap cleanup EXIT
	$(COMPOSE_TEST) up -d --build --wait --wait-timeout 120 db server
	$(COMPOSE_TEST) run --rm tests-python

.PHONY: test-integration-typescript
test-integration-typescript:
	cleanup() { $(COMPOSE_TEST) down -v --remove-orphans 2>/dev/null || true; }
	trap cleanup EXIT
	$(COMPOSE_TEST) up -d --build --wait --wait-timeout 120 db server
	$(COMPOSE_TEST) run --rm tests-typescript

.PHONY: test-all
test-all: test test-integration
	@echo "$(GREEN)✓ All tests passed$(RESET)"

# ═══════════════════════════════════════════════════════════════════
# QUALITY CHECKS
# ═══════════════════════════════════════════════════════════════════

.PHONY: lint
lint: proto-lint server-lint sdk-python-lint ts-lint
	@echo "$(GREEN)✓ All linters passed$(RESET)"

.PHONY: ci-check
ci-check: proto-check go-work-check lint test
	@echo "$(GREEN)✓ All CI checks passed$(RESET)"

# ═══════════════════════════════════════════════════════════════════
# DEVELOPMENT
# ═══════════════════════════════════════════════════════════════════

.PHONY: dev
dev: proto api-client
	@echo "$(CYAN)Starting development environment...$(RESET)"
	$(COMPOSE_DEV) up -d db
	./scripts/wait-for-it.sh localhost 5432 30
	$(COMPOSE_DEV) run --rm migrate up
	@echo "$(GREEN)✓ Database ready$(RESET)"
	trap 'kill 0' SIGINT
	(cd server && go run $(LDFLAGS) ./cmd/continua-server) &
	(./scripts/wait-for-it.sh localhost 8243 30 && pnpm --filter @continua/web dev) &
	wait

# ═══════════════════════════════════════════════════════════════════
# DOCKER
# ═══════════════════════════════════════════════════════════════════

.PHONY: docker-build
docker-build:
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		-f docker/Dockerfile.server \
		-t continua-server:$(VERSION) \
		-t continua-server:latest \
		.
	docker build -f docker/Dockerfile.web -t continua-web:latest .

.PHONY: docker-up
docker-up:
	$(COMPOSE_DEV) up -d

.PHONY: docker-down
docker-down:
	$(COMPOSE_DEV) down

# ═══════════════════════════════════════════════════════════════════
# CLEAN
# ═══════════════════════════════════════════════════════════════════

.PHONY: clean
clean: proto-clean
	rm -rf bin/
	rm -rf apps/web/.next/
	rm -rf packages/api-client/dist/
	rm -rf sdks/typescript/dist/
	pnpm clean || true
	@echo "$(GREEN)✓ Clean complete$(RESET)"
