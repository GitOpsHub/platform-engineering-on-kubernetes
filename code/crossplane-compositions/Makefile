# Crossplane XRDs, Compositions and claims. Nothing here needs cloud credentials.
#
#   make lint         parse every YAML file (multi-document aware) with PyYAML via uv
#   make render       run `crossplane render` for both example claims (needs Docker)
#   make test         lint + render, and fail if any function reports a fatal result
#   make validate     server-side dry-run of the XRDs and Compositions (needs a cluster)
#   make all          lint + test
#
# Lint needs uv. Render needs the crossplane CLI and Docker. Validate needs kubectl pointed at
# a cluster that already has Crossplane installed (see README.md, Install).

CROSSPLANE ?= crossplane
KUBECTL    ?= kubectl
FUNCTIONS  := functions/functions.yaml

YAML_FILES := $(shell find . -name '*.yaml' -not -path './.git/*' | sort)

# RENDER: run one claim through its Composition. $(1) = claim, $(2) = composition.
define RENDER
$(CROSSPLANE) render $(1) $(2) $(FUNCTIONS) --include-function-results
endef

# RENDER_CHECK: same render, but capture the output and fail on any fatal function result.
define RENDER_CHECK
@out=$$($(CROSSPLANE) render $(1) $(2) $(FUNCTIONS) --include-function-results) || exit 1; \
if echo "$$out" | grep -q 'SEVERITY_FATAL'; then \
  echo "FAIL: fatal function result while rendering $(1)"; echo "$$out"; exit 1; \
fi; \
echo "ok: $(1) renders without fatal results"
endef

.PHONY: help lint render test validate all

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "}; {printf "  %-12s %s\n", $$1, $$2}'

lint: ## Parse every YAML file and check each document has apiVersion and kind
	uv run --quiet --with pyyaml python hack/lint_yaml.py $(YAML_FILES)

render: ## Render each example claim through its Composition with the real functions
	$(call RENDER,claims/xenvironment.yaml,compositions/environment-kubernetes.yaml)
	$(call RENDER,claims/xdatabase.yaml,compositions/database-aws.yaml)

test: lint ## Lint, then render and fail on any SEVERITY_FATAL function result
	$(call RENDER_CHECK,claims/xenvironment.yaml,compositions/environment-kubernetes.yaml)
	$(call RENDER_CHECK,claims/xdatabase.yaml,compositions/database-aws.yaml)

validate: ## Server-side dry-run of XRDs and Compositions against the current cluster
	$(KUBECTL) apply --dry-run=server -f xrd/
	$(KUBECTL) apply --dry-run=server -f compositions/

all: lint test ## Lint and test
