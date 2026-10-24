# developer-tools

Developer-facing tooling:

- **Backstage** (`backstage` subchart, `2.10.2`, https://backstage.github.io/charts), toggled with `backstage.enabled`.
- **platform-cli-docs** ConfigMap, built from `files/*.md`. It contains the `platform-cli` command reference.

## Install

```bash
make -C code/helm-charts deps
helm lint --strict code/helm-charts/developer-tools
helm upgrade --install developer-tools code/helm-charts/developer-tools \
  --namespace developer-tools --create-namespace --wait
helm test developer-tools --namespace developer-tools
```

## Values

| Key | Default | Notes |
| --- | ------- | ----- |
| `backstage.enabled` | `true` | Keys under `backstage:` go to the subchart. Set `image` to your portal build. |
| `platformCli.docs.enabled` | `true` | Render the ConfigMap. |
| `platformCli.docs.name` | `platform-cli-docs` | ConfigMap name. |

## Updating the CLI docs

Edit `files/platform-cli.md` and upgrade the release. The ConfigMap is rebuilt from the file, so there is no
second copy to keep in sync. Keep the command tables in step with `code/platform-cli/commands/`.

## Tests

- `test-platform-cli-docs`: checks that the ConfigMap is mounted and non-empty.
- `test-backstage`: calls the Backstage liveness endpoint. It assumes the subchart's default Service name
  `<release>-backstage` on port 7007. Adjust it if you change `fullnameOverride`.
