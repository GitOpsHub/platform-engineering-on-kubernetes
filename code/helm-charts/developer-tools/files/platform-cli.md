# platform-cli

`platform-cli` is the command-line front end for the platform APIs. It talks to `platform-api`, which
runs each request as you (impersonation), so Kubernetes RBAC decides what you may do.

## Configuration

| Command | Purpose |
| ------- | ------- |
| `platform-cli config view` | Show the effective configuration. |
| `platform-cli config path` | Print the configuration file location. |
| `platform-cli config set KEY VALUE` | Set a configuration value, for example the API URL. |
| `platform-cli version` | Print the CLI version. |

## Tenants

| Command | Purpose |
| ------- | ------- |
| `platform-cli tenant create NAME --display-name NAME` | Create a Tenant. Requires membership in the platform admin group. |
| `platform-cli tenant list` | List Tenants you can see. |
| `platform-cli tenant get NAME` | Show one Tenant. |
| `platform-cli tenant delete NAME` | Delete a Tenant. Requires membership in the platform admin group. |

## Environments

| Command | Purpose |
| ------- | ------- |
| `platform-cli env create NAME --tenant TENANT` | Create an Environment inside a Tenant. |

## Workloads

| Command | Purpose |
| ------- | ------- |
| `platform-cli workload create NAME --image IMAGE` | Deploy a container image as a Workload. |

## Output

Commands print a table by default. The formatters are `table`, `json` and `yaml`; use `json` or `yaml` in scripts.
