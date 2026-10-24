TF       ?= terraform
EXAMPLES := $(wildcard examples/*)
MODULES  := networking/aws networking/gcp networking/azure eks gke aks addons

.PHONY: all fmt fmt-check validate docs clean

all: fmt-check validate

fmt:
	$(TF) fmt -recursive

fmt-check:
	$(TF) fmt -recursive -check -diff

validate:
	@set -e; for e in $(EXAMPLES); do \
		echo "==> $$e"; \
		$(TF) -chdir=$$e init -backend=false -input=false >/dev/null; \
		$(TF) -chdir=$$e validate; \
	done

docs:
	@for m in $(MODULES); do \
		terraform-docs markdown table --output-file README.md --output-mode inject --hide modules --sort-by required $$m; \
	done

clean:
	rm -rf examples/*/.terraform examples/*/.terraform.lock.hcl
