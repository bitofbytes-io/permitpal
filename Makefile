.DEFAULT_GOAL := help

BIN_DIR ?= bin
PORT ?= 4600
APP_ENV ?= development
DATA_STORE ?= memory
export DATABASE_URL
export PERMITPAL_USERS
export PERMITPAL_USERS_FILE
export SESSION_SECRET

REGISTRY ?= registry.tail209cfc.ts.net
IMAGE_REPO ?= permitpal
TAG ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)
PLATFORMS ?= linux/amd64,linux/arm64/v8

-include local.mk

.PHONY: help templ dev run run-postgres build test migrate migrate-down migrate-status docker-build docker-buildx clean

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make <target>\n\nTargets:\n"} /^[a-zA-Z0-9_.-]+:.*##/ {printf "  %-20s %s\n", $$1, $$2} END {printf "\n"}' $(MAKEFILE_LIST)

templ: ## Generate Go code from templ files
	go run github.com/a-h/templ/cmd/templ@$$(go list -m -f '{{.Version}}' github.com/a-h/templ) generate

dev: templ ## Run local visual preview with memory storage and no database
	@test -n "$$PERMITPAL_USERS$$PERMITPAL_USERS_FILE" || (echo "PERMITPAL_USERS_FILE or PERMITPAL_USERS must be set in local.mk or the environment" >&2; exit 1)
	@test -n '$(SESSION_SECRET)' || (echo "SESSION_SECRET must be set in local.mk or the environment" >&2; exit 1)
	APP_ENV=development DATA_STORE=memory PORT=$(PORT) go run ./cmd/permitpal

run: dev ## Alias for local preview

run-postgres: templ ## Run locally against Postgres
	@test -n "$$PERMITPAL_USERS$$PERMITPAL_USERS_FILE" || (echo "PERMITPAL_USERS_FILE or PERMITPAL_USERS must be set in local.mk or the environment" >&2; exit 1)
	@test -n '$(SESSION_SECRET)' || (echo "SESSION_SECRET must be set in local.mk or the environment" >&2; exit 1)
	@test -n '$(DATABASE_URL)' || (echo "DATABASE_URL must be set in local.mk or the environment" >&2; exit 1)
	APP_ENV=$(APP_ENV) DATA_STORE=postgres PORT=$(PORT) DATABASE_URL="$(DATABASE_URL)" go run ./cmd/permitpal

build: templ ## Build the production binary
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/permitpal ./cmd/permitpal

test: templ ## Run Go tests
	go test ./...

migrate: ## Apply Postgres migrations with goose
	@test -n '$(DATABASE_URL)' || (echo "DATABASE_URL must be set in local.mk or the environment" >&2; exit 1)
	goose -dir migrations postgres "$(DATABASE_URL)" up

migrate-down: ## Roll back one Postgres migration
	@test -n '$(DATABASE_URL)' || (echo "DATABASE_URL must be set in local.mk or the environment" >&2; exit 1)
	goose -dir migrations postgres "$(DATABASE_URL)" down

migrate-status: ## Show Postgres migration status
	@test -n '$(DATABASE_URL)' || (echo "DATABASE_URL must be set in local.mk or the environment" >&2; exit 1)
	goose -dir migrations postgres "$(DATABASE_URL)" status

docker-build: templ ## Build the Docker image locally
	docker build -t $(REGISTRY)/$(IMAGE_REPO):$(TAG) .

# METADATA_FILE, when set, receives buildx's build metadata; CI reads the pushed digest from it.
docker-buildx: templ ## Build and push a multi-arch Docker image
	docker buildx build \
		--platform $(PLATFORMS) \
		--tag $(REGISTRY)/$(IMAGE_REPO):$(TAG) \
		--tag $(REGISTRY)/$(IMAGE_REPO):latest \
		$(if $(METADATA_FILE),--metadata-file "$(METADATA_FILE)") \
		--push \
		.

clean: ## Remove local build outputs
	rm -rf $(BIN_DIR)
