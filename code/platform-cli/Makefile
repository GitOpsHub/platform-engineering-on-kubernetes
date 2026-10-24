# platform-cli - build, test and package targets

BINARY_NAME := platform-cli
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT      ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME  ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS     := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildTime=$(BUILD_TIME)

GO          ?= go
CGO_ENABLED ?= 0
GOOS        ?= $(shell $(GO) env GOOS)
GOARCH      ?= $(shell $(GO) env GOARCH)

IMAGE       ?= ghcr.io/gitopshub/$(BINARY_NAME)
TAG         ?= $(VERSION)

.DEFAULT_GOAL := help

##@ Development

.PHONY: run
run: ## Run the CLI locally, e.g. make run ARGS="tenant list"
	$(GO) run -ldflags "$(LDFLAGS)" . $(ARGS)

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
build: ## Build the binary into bin/
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) .

.PHONY: test
test: ## Run unit tests with the race detector and coverage
	$(GO) test -race -coverprofile=coverage.out ./...

.PHONY: coverage
coverage: test ## Open an HTML coverage report
	$(GO) tool cover -html=coverage.out -o coverage.html

##@ Container

.PHONY: docker-build
docker-build: ## Build the container image (CLI in a distroless image, for CI jobs)
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg BUILD_TIME=$(BUILD_TIME) \
		-t $(IMAGE):$(TAG) .

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf bin/ coverage.out coverage.html

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make <target>\n"} \
		/^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2 } \
		/^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)
