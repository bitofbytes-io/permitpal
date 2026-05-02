.DEFAULT_GOAL := help

BIN_DIR ?= bin
PORT ?= 4600
APP_ENV ?= development
DATA_STORE ?= memory
PERMITPAL_PASSWORD ?= permitpal
DATABASE_URL ?= postgres://permitpal:permitpal@localhost:5432/permitpal?sslmode=disable
export DATABASE_URL

NORMALIZE_DATABASE_URL = python3 -c 'import os, urllib.parse as u; url = os.environ.get("DATABASE_URL", ""); scheme, sep, rest = url.partition("://"); authority, at, tail = rest.rpartition("@"); valid = bool(sep and at and ":" in authority); user, password = authority.split(":", 1) if valid else ("", ""); user = u.quote(u.unquote(user), safe=""); password = u.quote(u.unquote(password), safe=""); print(scheme + "://" + user + ":" + password + "@" + tail if valid else url)'

REGISTRY ?= registry.bitofbytes.io
IMAGE_REPO ?= permitpal
TAG ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)
PLATFORMS ?= linux/amd64,linux/arm64/v8

-include local.mk

.PHONY: help templ tail-watch tail-prod dev run run-postgres build test migrate migrate-down migrate-status docker-build docker-buildx clean

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make <target>\n\nTargets:\n"} /^[a-zA-Z0-9_.-]+:.*##/ {printf "  %-20s %s\n", $$1, $$2} END {printf "\n"}' $(MAKEFILE_LIST)

templ: ## Generate Go code from templ files
	templ generate

tail-watch: ## Rebuild CSS on changes when tailwindcss is available
	tailwindcss -i ./tailwind/styles.css -o ./static/styles.css --watch

tail-prod: ## Build CSS for production, or copy source CSS if Tailwind is unavailable
	@if command -v tailwindcss >/dev/null 2>&1; then \
		tailwindcss -i ./tailwind/styles.css -o ./static/styles.css; \
	else \
		cp ./tailwind/styles.css ./static/styles.css; \
	fi

dev: templ tail-prod ## Run local visual preview with memory storage and no database
	APP_ENV=development DATA_STORE=memory PORT=$(PORT) PERMITPAL_PASSWORD=$(PERMITPAL_PASSWORD) go run ./cmd/permitpal

run: dev ## Alias for local preview

run-postgres: templ tail-prod ## Run locally against Postgres
	APP_ENV=$(APP_ENV) DATA_STORE=postgres PORT=$(PORT) DATABASE_URL="$$($(NORMALIZE_DATABASE_URL))" PERMITPAL_PASSWORD=$(PERMITPAL_PASSWORD) go run ./cmd/permitpal

build: templ tail-prod ## Build the production binary
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/permitpal ./cmd/permitpal

test: templ ## Run Go tests
	go test ./...

migrate: ## Apply Postgres migrations with goose
	goose -dir migrations postgres "$$($(NORMALIZE_DATABASE_URL))" up

migrate-down: ## Roll back one Postgres migration
	goose -dir migrations postgres "$$($(NORMALIZE_DATABASE_URL))" down

migrate-status: ## Show Postgres migration status
	goose -dir migrations postgres "$$($(NORMALIZE_DATABASE_URL))" status

docker-build: templ tail-prod ## Build the Docker image locally
	docker build -t $(REGISTRY)/$(IMAGE_REPO):$(TAG) .

docker-buildx: templ tail-prod ## Build and push a multi-arch Docker image
	docker buildx build \
		--platform $(PLATFORMS) \
		--tag $(REGISTRY)/$(IMAGE_REPO):$(TAG) \
		--tag $(REGISTRY)/$(IMAGE_REPO):latest \
		--push \
		.

clean: ## Remove local build outputs
	rm -rf $(BIN_DIR)
