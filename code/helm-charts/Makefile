# Helm charts for the platform. Run from the repository root with: make -C code/helm-charts <target>
#
#   make sync-crds   copy generated CRDs from the operators into platform-core/crds/
#   make deps        helm dependency build for charts with dependencies (needs network)
#   make lint        helm lint --strict for every chart
#   make template    helm template for every chart (renders with default values)
#   make all         sync-crds is NOT part of all, because it changes files. Run it explicitly.

HELM     ?= helm
CHARTS   := platform-core platform-addons developer-tools
DEP_CHARTS := platform-addons developer-tools

OPERATORS := tenant-operator environment-operator workload-operator

.PHONY: all sync-crds deps lint template clean

all: deps lint template

# Copy whatever each operator generates. Operators without generated CRDs keep their placeholder.
sync-crds:
	@set -e; for op in $(OPERATORS); do \
		src=../$$op/config/crd/bases; \
		if ls $$src/*.yaml >/dev/null 2>&1; then \
			cp $$src/*.yaml platform-core/crds/; \
			echo "synced CRDs from $$op"; \
		else \
			echo "no generated CRDs in $$op; placeholder kept"; \
		fi; \
	done

deps:
	@set -e; for c in $(DEP_CHARTS); do \
		echo "==> $$c"; \
		$(HELM) dependency build $$c; \
	done

lint:
	@set -e; for c in $(CHARTS); do \
		echo "==> lint $$c"; \
		$(HELM) lint --strict $$c; \
	done

template:
	@set -e; for c in $(CHARTS); do \
		echo "==> template $$c"; \
		$(HELM) template test $$c --set-string 'platformApi.impersonation.users[0]=ci-template' >/dev/null; \
	done

clean:
	rm -rf $(addsuffix /charts,$(DEP_CHARTS))
