# ${{ values.name }}

${{ values.description }}

**Owner:** `${{ values.owner }}` · **System:** `${{ values.system }}`

## How it runs

This repository ships one object the platform understands: [`deploy/workload.yaml`](deploy/workload.yaml),
a `Workload` in `platform.mycompany.io/v1alpha1`. A GitOps tool (Lab 02) applies it, and the
platform controller turns it into the Kubernetes objects the service needs.

Change the image tag, replicas or port in `deploy/workload.yaml` and commit. Do not write
Deployments or Services by hand.

## Catalog

The component is described in [`catalog-info.yaml`](catalog-info.yaml). Edit it when the owner,
lifecycle or dependencies change.
