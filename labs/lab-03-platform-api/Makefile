# Makefile for the WebApp platform API (Lab 03).
#
#   make help        list targets
#   make test        unit tests (no cluster needed)
#   make run         run the controller on your laptop, against the current kube context
#   make install     install the WebApp CRD into the cluster
#   make deploy      build the image, load it into kind, deploy the controller
#   make undeploy    remove the controller and its RBAC
#   make uninstall   remove the CRD (this deletes every WebApp)

SHELL := bash
.SHELLFLAGS := -eu -o pipefail -c

# Image and cluster used by `make deploy`. Override on the command line, e.g.
#   make deploy KIND_CLUSTER=my-cluster
IMG                      ?= webapp-controller:dev
KIND_CLUSTER             ?= platform-engineering
NAMESPACE                ?= platform-system

# controller-gen generates DeepCopy methods, the CRD and RBAC from the markers in the code.
CONTROLLER_TOOLS_VERSION ?= v0.20.1
LOCALBIN                 ?= $(CURDIR)/bin
CONTROLLER_GEN           ?= $(LOCALBIN)/controller-gen

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z_-]+:.*## / {printf "  %-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

##@ Development

.PHONY: fmt
fmt: ## Format all Go code
	go fmt ./...

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: test
test: ## Run unit tests (uses the fake client, no cluster needed)
	go test ./... -count=1 -cover

.PHONY: build
build: fmt vet ## Build the controller binary into bin/manager
	mkdir -p bin
	go build -o bin/manager ./cmd

.PHONY: run
run: ## Run the controller locally using your kubeconfig (install the CRD first)
	go run ./cmd

##@ Code generation

.PHONY: generate
generate: controller-gen ## Generate DeepCopy methods into api/v1alpha1/zz_generated.deepcopy.go
	$(CONTROLLER_GEN) object paths="./api/..."

.PHONY: manifests
manifests: controller-gen ## Generate the CRD (crds/) and the ClusterRole (deploy/role.yaml) from markers
	$(CONTROLLER_GEN) crd paths="./api/..." output:crd:artifacts:config=crds
	$(CONTROLLER_GEN) rbac:roleName=webapp-controller paths="./controllers/..." output:rbac:artifacts:config=deploy

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen into bin/ (pinned version)

$(CONTROLLER_GEN):
	mkdir -p $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

##@ Cluster

.PHONY: install
install: ## Install the WebApp CRD into the current cluster
	kubectl apply -f crds/

.PHONY: uninstall
uninstall: ## Remove the WebApp CRD (deletes every WebApp in the cluster)
	kubectl delete -f crds/ --ignore-not-found

.PHONY: docker-build
docker-build: ## Build the controller image
	docker build -t $(IMG) .

.PHONY: kind-load
kind-load: docker-build ## Load the image into the Lab 01 kind cluster (k3d: k3d image import)
	kind load docker-image $(IMG) --name $(KIND_CLUSTER)

.PHONY: deploy
deploy: install kind-load ## Build, load and deploy the controller into the cluster
	kubectl kustomize deploy | sed "s#image: controller:dev#image: $(IMG)#" | kubectl apply -f -
	kubectl -n $(NAMESPACE) rollout status deployment/webapp-controller --timeout=120s

.PHONY: undeploy
undeploy: ## Remove the controller and its RBAC (the CRD and WebApps are kept)
	kubectl kustomize deploy | kubectl delete --ignore-not-found -f -

.PHONY: clean
clean: ## Remove build outputs
	rm -rf bin
