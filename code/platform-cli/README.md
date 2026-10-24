# platform-cli

Command-line interface for the Internal Developer Platform. It manages Tenants,
Environments and Workloads (`platform.mycompany.io/v1alpha1`) directly through the
Kubernetes API, so the cluster's RBAC applies to whoever runs the command.

```sh
platform-cli tenant list
platform-cli tenant create team-c --display-name "Team C" --owner carol --quota-cpu 4
platform-cli env create pr-42 -n team-c-dev --tenant team-c --type preview --ttl 24h
platform-cli workload create checkout -n team-c-dev --image ghcr.io/acme/checkout:1.4.2 \
  --replicas 2 --env LOG_LEVEL=info --trait autoscaling='{"maxReplicas":10}'
platform-cli workload list -o yaml
platform-cli config set namespace team-c-dev
```

## Commands

| Command | Purpose |
|---|---|
| `tenant list \| get \| create \| delete` | Cluster-scoped Tenants |
| `env list \| get \| create \| delete` | Environments (aliases: `environment`, `envs`) |
| `workload list \| get \| create \| delete` | Workloads (alias: `wl`) |
| `config view \| set \| path` | Inspect and persist settings |
| `version` | Build and runtime information |

`list` for namespaced kinds spans all namespaces unless `-n` or a configured namespace
is set. `get`, `create` and `delete` need a namespace.

## Configuration

Settings resolve in this order, lowest to highest precedence:

1. `~/.platform/config.yaml` (or the path in `$PLATFORM_CONFIG`, or `--config`)
2. environment variables: `PLATFORM_KUBECONFIG`, `PLATFORM_CONTEXT`,
   `PLATFORM_NAMESPACE`, `PLATFORM_OUTPUT`
3. flags: `--kubeconfig`, `--context`, `-n/--namespace`, `-o/--output`

```yaml
# ~/.platform/config.yaml
kubeconfig: ~/.kube/config
context: platform-prod
namespace: team-c-dev
output: table      # table | json | yaml
```

Unknown keys in the file are rejected. The file is written with mode 0600.

## Output

`-o table` (default), `-o json` or `-o yaml`. JSON and YAML print the full object
or `*List` document, so they are safe to pipe into other tools.

## Develop

```sh
make help
make test     # go test -race with coverage
make build    # bin/platform-cli
```

## Notes

- Workload `resources` uses the Kubernetes requests/limits shape
  (`--cpu-request`, `--memory-request`, `--cpu-limit`, `--memory-limit`).
- Create commands send the object as-is. Validation and defaulting are the job of the
  admission webhook and the operators.
