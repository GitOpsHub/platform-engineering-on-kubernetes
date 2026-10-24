# service-catalog - build, test and package targets

BINARY_NAME := catalog
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT      ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME  ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS     := -s -w

GO          ?= go
CGO_ENABLED ?= 0

IMAGE       ?= ghcr.io/gitopshub/service-catalog
TAG         ?= $(VERSION)

.DEFAULT_GOAL := help

##@ Development

.PHONY: run
run: ## Run the catalog CLI, e.g. make run ARGS="list"
	$(GO) run ./cmd/catalog $(ARGS)

.PHONY: fmt
fmt: ## Format code
	$(GO) fmt ./...

.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

.PHONY: lint
lint: vet ## Run golangci-lint (falls back to go vet if not installed)
	@if command -v golangci-lint >/dev/null 2>&1; then golangci-lint run ./...; \
	else echo "golangci-lint not installed; ran go vet only"; fi

.PHONY: tidy
tidy: ## go mod tidy + verify
	$(GO) mod tidy
	$(GO) mod verify

##@ Build & test

.PHONY: build
build: ## Build the catalog CLI into bin/
	CGO_ENABLED=$(CGO_ENABLED) $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) ./cmd/catalog

.PHONY: test
test: ## Run unit tests with the race detector and coverage
	$(GO) test -race -coverprofile=coverage.out ./...

.PHONY: coverage
coverage: test ## Open an HTML coverage report
	$(GO) tool cover -html=coverage.out -o coverage.html

##@ Container

.PHONY: docker-build
docker-build: ## Build the container image
	docker build -t $(IMAGE):$(TAG) .

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf bin/ coverage.out coverage.html

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make <target>\n"} \
		/^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2 } \
		/^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)
