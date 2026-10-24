# Reference Terraform/OpenTofu modules. Targets run locally and never touch cloud credentials.
#
#   make fmt        format every .tf file
#   make validate   init (no backend) and validate every module and example
#   make docs       regenerate the Inputs/Outputs/Resources tables in each module README
#   make all        fmt-check + validate
#
# The CLI is auto-detected: terraform if installed, otherwise tofu. Override with TF=tofu.

TF       ?= $(shell command -v terraform >/dev/null 2>&1 && echo terraform || echo tofu)
MODULES  := networking/aws networking/gcp networking/azure eks gke aks addons
EXAMPLES := examples/eks-complete examples/gke-complete examples/aks-complete

.PHONY: all fmt fmt-check validate docs clean

all: fmt-check validate

fmt:
	$(TF) fmt -recursive

fmt-check:
	$(TF) fmt -recursive -check -diff

validate:
	@set -e; for d in $(MODULES) $(EXAMPLES); do \
		echo "==> $$d"; \
		$(TF) -chdir=$$d init -backend=false -input=false -no-color >/dev/null; \
		$(TF) -chdir=$$d validate -no-color; \
	done

docs:
	@set -e; for m in $(MODULES); do \
		terraform-docs markdown table --output-file README.md --output-mode inject --hide modules --sort-by required $$m; \
	done

# Keeps .terraform.lock.hcl files in examples (they pin provider versions).
clean:
	find . -type d -name .terraform -prune -exec rm -rf {} +
