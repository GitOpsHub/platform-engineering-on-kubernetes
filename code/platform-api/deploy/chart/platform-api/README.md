# platform-api Helm chart

Deploys platform-api, the REST front door for Tenants, Environments and Workloads.

## Impersonation scope

platform-api does not use its own identity to touch platform resources. For each
request it builds a client that impersonates the authenticated caller (`users` and
`groups` of the verified token). Cluster RBAC then decides what that caller can do.

The ServiceAccount's ClusterRole grants only `impersonate`. Its scope is set in
`values.yaml`:

| Value | Meaning |
|---|---|
| `impersonation.users` | Subjects platform-api may act as. Rendered as `resourceNames` on the `users` rule. **Required** unless `allowAnyUser` is true. |
| `impersonation.groups` | Groups platform-api may act as, exactly as listed. Rendered as `resourceNames` on the `groups` rule. **Must be non-empty.** Default `[platform-admins]`. |
| `impersonation.allowAnyUser` | Explicit, unscoped opt-in for impersonating any user. Default `false`. |
| `platformAdminGroup` | Group whose members may create and delete Tenants. |

**Admin check vs. group allowlist.** The Tenant create and delete gate reads the caller's
own groups from their verified token. It does not read the groups platform-api forwards.
So an admin is recognised even when `platformAdminGroup` is not in `impersonation.groups`.
Listing the admin group in `impersonation.groups` is only needed if members should act as
that group in cluster RBAC, for example to read objects that RBAC grants to the group.

The same keys appear under `platformApi.impersonation` in the platform-core umbrella chart, with the same rendering rules.

### Rules that are enforced

- **Never `system:*`.** The chart fails to render if any `users`, `groups` or
  `platformAdminGroup` value starts with `system:`. That covers `system:masters`,
  `system:nodes` and every other system group. RBAC cannot express "anything except
  `system:*`", so the allowlist is the real scoping.
- **Server-side filtering.** Before it impersonates, platform-api refuses a principal
  whose subject or groups start with `system:` (HTTP 403, no client built). It drops
  any group that is not in the `--impersonate-groups` list, which the chart renders from
  the same values. A dropped group is logged, not forwarded.
- **Residual broad permission: `impersonate` on `users`.** Kubernetes cannot scope
  this to a pattern, so with `allowAnyUser=false` the scope is only the listed subjects.
  It is still the most powerful permission in the platform. It is accepted because
  the alternative (every portal or tenant holding its own cluster credentials) is worse.
  Record it in the platform threat model.

### Audit and alerting

Every impersonation is logged as one JSON line with `"audit": true`:

- `impersonating`: `subject`, the groups actually forwarded, and `droppedGroups`
- `refused impersonation of a reserved identity` (level `WARN`): the subject and groups
  that were refused

Alert on any `WARN` line with `audit: true`. Alert on `droppedGroups` appearing for a
subject that is not expected to hold those groups. Logs leave the cluster through the
usual log pipeline, so keep them retained.

## Install

The chart fails to render without an impersonation scope, which is intended:

```sh
helm upgrade --install platform-api deploy/chart/platform-api -n platform-system \
  --set auth.existingSecret=platform-api-tokens \
  --set impersonation.groups='{tenant-payments,tenant-checkout}' \
  --set impersonation.users='{}' --set impersonation.allowAnyUser=true   # or list subjects
```

`ci/lint-values.yaml` supplies placeholder values for `make helm-lint` and `make helm-template`.
