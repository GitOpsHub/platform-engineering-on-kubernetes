# service-catalog

A small service catalog for the platform. It holds scaffolding templates for new
services and turns them into a repository skeleton that already has what the platform
expects: a Dockerfile, a `Workload` custom resource and a Backstage-style
`catalog-info.yaml`.

```sh
catalog list
catalog describe go-http-service
catalog scaffold go-http-service --set name=checkout --set owner=payments --set namespace=payments-prod
catalog scaffold static-site --set name=landing --set owner=growth --dry-run
catalog --templates ./team-templates list      # add a team's own templates
```

## Built-in templates

| Template | Produces |
|---|---|
| `go-http-service` | `Dockerfile` (distroless), `main.go`, `go.mod`, `workload.yaml`, `catalog-info.yaml` |
| `static-site` | `Dockerfile` (unprivileged nginx), `site/index.html`, `workload.yaml`, `catalog-info.yaml` |

The `workload.yaml` output is a `platform.mycompany.io/v1alpha1` `Workload`, so
`kubectl apply -f workload.yaml` (or GitOps) hands it to the workload operator.

## Template format

A template is a directory with a `template.yaml` and files ending in `.tmpl`:

```yaml
name: go-http-service          # DNS label; also the CLI name
title: Go HTTP service
parameters:
  - name: owner                # lowerCamel; available as {{ .owner }}
    required: true
    pattern: '^[a-z0-9][a-z0-9-]*$'
  - name: lifecycle
    default: experimental
    enum: [experimental, production, deprecated]
```

Every `.tmpl` file is executed with `text/template` (missing keys are errors) and
written without the suffix. Other files are copied as-is. `quote`, `lower` and `upper`
are available. Parameter validation reports all problems at once and rejects unknown
keys.

## Layout

| Path | Purpose |
|---|---|
| `catalog.go` | Template metadata, parameter validation, rendering, safe tree writing |
| `templates/` | Built-in template skeletons, embedded with `go:embed` |
| `registry/` | Loads built-in and directory-based templates; rejects duplicate names |
| `cli/` | `list`, `describe`, `scaffold` commands |
| `cmd/catalog/` | Entry point |

## Develop

```sh
make help
make test    # go test -race ./...
make run ARGS="list"
```

## Safety

`scaffold` refuses to overwrite existing files unless `--force` is given. Output paths
must stay inside the target directory, and nothing is written when any check fails.
