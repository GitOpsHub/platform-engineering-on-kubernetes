# policy-engine - build, test and package targets

BINARY_NAME := policy-engine
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS     := -s -w -X main.version=$(VERSION)

GO          ?= go
CGO_ENABLED ?= 0
OPA         ?= opa

IMAGE       ?= ghcr.io/gitopshub/$(BINARY_NAME)
TAG         ?= $(VERSION)

.DEFAULT_GOAL := help

##@ Development

.PHONY: run
run: ## Run the webhook locally (needs TLS files; see README)
	$(GO) run -ldflags "$(LDFLAGS)" ./cmd/policy-engine --tls-cert=$${TLS_CERT:-certs/tls.crt} --tls-key=$${TLS_KEY:-certs/tls.key} --log-level=debug

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

##@ Policies

.PHONY: opa-test
opa-test: ## Run Rego tests with the opa CLI (skipped when opa is not installed)
	@if command -v $(OPA) >/dev/null 2>&1; then $(OPA) test policies/ -v; \
	else echo "opa not installed; the Go test suite still runs the Rego tests (make test)"; fi

##@ Build & test

.PHONY: build
build: ## Build the webhook binary into bin/
	CGO_ENABLED=$(CGO_ENABLED) $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) ./cmd/policy-engine

.PHONY: test
test: ## Run unit tests (including the Rego suite) with the race detector and coverage
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
