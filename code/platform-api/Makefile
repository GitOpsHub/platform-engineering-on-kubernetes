# Platform API - build, test and deploy targets

BINARY_NAME := platform-api
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT      ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME  ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS     := -s -w -X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME)

GO          ?= go
CGO_ENABLED ?= 0

IMAGE       ?= ghcr.io/gitopshub/$(BINARY_NAME)
TAG         ?= $(VERSION)

HELM        ?= helm
NAMESPACE   ?= platform-system
CHART       := deploy/chart/$(BINARY_NAME)

.DEFAULT_GOAL := help

##@ Development

.PHONY: run
run: ## Run locally against the current kubeconfig (DEV ONLY: --insecure-skip-authz, no tokens)
	$(GO) run -ldflags "$(LDFLAGS)" . --kubeconfig=$${KUBECONFIG:-$$HOME/.kube/config} --log-level=debug --insecure-skip-authz

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
	CGO_ENABLED=$(CGO_ENABLED) $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) .

.PHONY: test
test: ## Run unit tests with the race detector and coverage
	$(GO) test -race -coverprofile=coverage.out ./...

.PHONY: coverage
coverage: test ## Open an HTML coverage report
	$(GO) tool cover -html=coverage.out -o coverage.html

##@ Container

.PHONY: docker-build
docker-build: ## Build the container image
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg BUILD_TIME=$(BUILD_TIME) \
		-t $(IMAGE):$(TAG) .

.PHONY: docker-push
docker-push: docker-build ## Push the container image
	docker push $(IMAGE):$(TAG)

##@ Deploy

.PHONY: helm-lint
helm-lint: ## Lint the Helm chart
	$(HELM) lint $(CHART) -f $(CHART)/ci/lint-values.yaml

.PHONY: helm-template
helm-template: ## Render the Helm chart
	$(HELM) template $(BINARY_NAME) $(CHART) -f $(CHART)/ci/lint-values.yaml --namespace $(NAMESPACE) --set image.repository=$(IMAGE) --set image.tag=$(TAG)

.PHONY: helm-install
helm-install: ## Install/upgrade the Helm release
	$(HELM) upgrade --install $(BINARY_NAME) $(CHART) --namespace $(NAMESPACE) --create-namespace \
		--set image.repository=$(IMAGE) --set image.tag=$(TAG) --wait --timeout 5m

.PHONY: helm-uninstall
helm-uninstall: ## Uninstall the Helm release
	$(HELM) uninstall $(BINARY_NAME) --namespace $(NAMESPACE)

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf bin/ coverage.out coverage.html

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make <target>\n"} \
		/^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2 } \
		/^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)
