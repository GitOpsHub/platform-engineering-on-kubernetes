# Secrets Rotation: Automated Patterns for Platforms

## Learning Objectives
- Classify secrets by how they rotate: static, dynamic (leased), certificate, and workload identity
- Configure External Secrets Operator refresh behavior so that source changes reach the cluster on time
- Use Vault dynamic secrets so that credentials are short-lived by design
- Restart or reload workloads automatically when a Secret or ConfigMap changes, with Reloader
- Automate certificate renewal with cert-manager and rotate a secret store's own credentials without downtime

## Prerequisites
- [External Secrets Operator](./external-secrets.md) and [Vault Integration](./vault-integration.md)
- Basic Kubernetes workload operations: Deployments, rollouts, and Secrets
- Helm 3, and a cluster with Prometheus if you want to run the monitoring examples

---

## Overview

Rotation is the process of replacing a credential before it is compromised or simply because it is old. A
platform that rotates secrets by hand will eventually skip a step, so the goal is to make rotation a
property of the system rather than an event someone remembers.

Rotation has three moving parts, and each needs its own automation:

1. **Issuance**: the source of truth creates a new credential (Vault, a cloud IAM, a certificate authority).
2. **Delivery**: the new value reaches the cluster (ESO, Vault Agent, the CSI driver, cert-manager).
3. **Consumption**: the workload starts using the new value (the application re-reads it, or it restarts).

Most outages during rotation happen in step 3. A Secret updates on time, and the application keeps using the
old value until its next restart, or it restarts all at once and drops connections. The patterns below address
all three steps.

```mermaid
graph LR
    Source[Source of truth<br/>Vault, cloud IAM, CA] -->|1. issue new value| Deliver[Delivery<br/>ESO, Vault Agent, CSI, cert-manager]
    Deliver -->|2. Secret or file updated| Consume[Consumption<br/>app re-read or restart]
    Consume -->|3. old credential unused| Revoke[Revoke or expire old value]
    Revoke -.->|next cycle| Source
```

### Rotation Patterns at a Glance

| Secret Type | Example | Recommended Pattern | Who Rotates It | Consumption Strategy |
|-------------|---------|---------------------|----------------|----------------------|
| Static credential in a secret store | API key in Vault KV | ESO `refreshInterval` + scheduled rotation job | Platform job | Reloader restart or app reload |
| Dynamic database credential | `database/creds/app` | Vault dynamic secrets with leases | Vault | Agent re-renders file; app reconnects |
| TLS certificate | Ingress or mTLS cert | cert-manager `Certificate` with `renewBefore` | cert-manager | Reloader restart, or app file watch |
| Workload identity token | Projected service account token | kubelet automatic rotation | kubelet | App re-reads the projected file |
| Store credential (ESO or Vault auth) | AppRole secret ID, cloud access key | Dual-credential overlap | Platform job | Update the store object, then revoke the old one |

---

## Pattern 1: ESO Refresh Intervals

ESO polls the provider and writes the value into the target `Secret` when it changes. The refresh behavior is
controlled on the `ExternalSecret`:

```yaml
# billing-api-externalsecret.yaml
apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata:
  name: billing-api-keys
  namespace: team-billing
  labels:
    platform.mycompany.io/tenant: billing
spec:
  refreshInterval: 5m
  refreshPolicy: Periodic
  secretStoreRef:
    kind: ClusterSecretStore
    name: vault-backend
  target:
    name: billing-api-keys
    creationPolicy: Owner
    deletionPolicy: Retain
  dataFrom:
    - extract:
        key: tenants/billing/api
```

| Field | Meaning | Rotation Guidance |
|-------|---------|-------------------|
| `refreshInterval` | How often ESO re-reads the source | Match the rotation window. A 5-minute interval means a rotated value reaches the cluster within about 5 minutes |
| `refreshPolicy` | `Periodic` re-reads on every interval; `OnChange` re-reads only when the `ExternalSecret` spec changes; `CreatedOnly` never re-reads | Use `Periodic` for rotated values; `OnChange` or `CreatedOnly` for values that must never change without a manifest change |
| `creationPolicy` | Ownership of the target `Secret` | `Owner` lets ESO clean up; use `Merge` only when another controller owns the Secret |
| `deletionPolicy` | What happens to the target `Secret` when the `ExternalSecret` is deleted | `Retain` keeps running workloads working during a migration |

### Forcing a Refresh

A rotation job that updates Vault can force an immediate sync instead of waiting for the interval:

```bash
kubectl -n team-billing annotate externalsecret billing-api-keys \
  reconcile.external-secrets.io/force="$(date +%s)" --overwrite
```

### Choosing an Interval

Shorter intervals reduce the window in which a stale value is in use, but they add load to the store. Keep a
few rules in mind:

- Rotations that happen daily can use a 5 to 15 minute interval.
- Dynamic or short-lived values should not come through ESO at all. Use Vault Agent instead (Pattern 2).
- Watch provider rate limits. Hundreds of `ExternalSecret` objects at a one-minute interval will hit cloud API
  quotas. Group related keys into one `ExternalSecret` and use the `dataFrom` form.

---

## Pattern 2: Vault Dynamic Secrets

Dynamic secrets flip the model. Instead of rotating a shared password, Vault creates a unique credential per
consumer with a lease. When the lease expires, Vault revokes the credential in the database. There is nothing
to rotate because nothing is long-lived.

```mermaid
sequenceDiagram
    participant Agent as Vault Agent (sidecar)
    participant Vault
    participant DB as PostgreSQL
    Agent->>Vault: login (Kubernetes auth)
    Vault-->>Agent: Vault token
    Agent->>Vault: read database/creds/billing
    Vault->>DB: CREATE ROLE v-billing-xyz with TTL
    Vault-->>Agent: username, password, lease_duration=1h
    Agent->>Agent: render template to /vault/secrets/db.env
    loop before lease expires
        Agent->>Vault: renew lease
    end
    Note over Agent,Vault: If renewal is no longer allowed, Agent requests a new credential
    Vault->>DB: REVOKE role when lease ends
```

### Configuring the Database Secrets Engine

```bash
# vault-database-setup.sh
vault secrets enable database

vault write database/config/billing-postgres \
  plugin_name=postgresql-database-plugin \
  connection_url="postgresql://{{username}}:{{password}}@billing-db.internal:5432/billing?sslmode=require" \
  allowed_roles="billing-app" \
  username="vault-admin" \
  password="${VAULT_DB_ADMIN_PASSWORD}"

# Rotate the admin password Vault itself uses, so that no human knows it
vault write -force database/rotate-root/billing-postgres

vault write database/roles/billing-app \
  db_name=billing-postgres \
  creation_statements="CREATE ROLE \"{{name}}\" WITH LOGIN PASSWORD '{{password}}' VALID UNTIL '{{expiration}}'; GRANT SELECT, INSERT, UPDATE ON ALL TABLES IN SCHEMA public TO \"{{name}}\";" \
  revocation_statements="DROP ROLE IF EXISTS \"{{name}}\";" \
  default_ttl=1h \
  max_ttl=24h
```

Three details matter here:

- `rotate-root` changes the password Vault uses to administer the database. After it runs, the bootstrap
  password exists only in Vault.
- `default_ttl` is the lease length handed out on each read; `max_ttl` is the hard ceiling after which renewal
  stops and a new credential must be issued.
- Grants belong in `creation_statements`. Keep them as narrow as the application allows.

### Consuming Dynamic Credentials

The Agent Injector already handles renewal and re-rendering. Point the template at the dynamic path:

```yaml
# billing-api-dynamic-db.yaml (pod template annotations excerpt)
vault.hashicorp.com/agent-inject: "true"
vault.hashicorp.com/role: "billing"
vault.hashicorp.com/agent-inject-secret-db.env: "database/creds/billing-app"
vault.hashicorp.com/agent-inject-template-db.env: |
  {{- with secret "database/creds/billing-app" -}}
  export DB_USER="{{ .Data.username }}"
  export DB_PASSWORD="{{ .Data.password }}"
  {{- end -}}
```

The policy for the role must allow `read` on `database/creds/billing-app`. The lease ID is available in the
agent logs, which helps when debugging.

### Lease Operations

```bash
# Inspect leases for a role
vault list sys/leases/lookup/database/creds/billing-app

# Revoke every outstanding credential for a role (incident response, or after a suspected leak)
vault lease revoke -prefix database/creds/billing-app
```

Revoking by prefix is the fastest containment step when a credential leaks. Applications reconnect on their
next attempt, because the agent has to fetch a new credential when its lease is gone.

### What the Application Must Do

Dynamic credentials only work if the application handles reconnection:

- Use a connection pool that can recreate connections when the password changes, or a pool that validates
  connections on borrow.
- Read the credential file again on reconnect. Do not cache the password at process start.
- Expect authentication errors during the handover window and retry with backoff.

If the application cannot handle this, use a shorter-lived static pattern instead (Pattern 1 with Reloader).

---

## Pattern 3: Reloader for Restart-on-Change

Many applications read configuration once at startup. A changed Secret does not change the running process.
[Reloader](https://github.com/stakater/Reloader) watches Secrets and ConfigMaps and triggers a rolling restart
of the workloads that reference them.

### Installing Reloader

```bash
helm repo add stakater https://stakater.github.io/stakater-charts
helm repo update

helm upgrade --install reloader stakater/reloader \
  --namespace reloader --create-namespace \
  --set reloader.watchGlobally=false
```

Setting `watchGlobally=false` keeps Reloader's view within the namespaces that opt in, which is the safer
default in a multi-tenant cluster.

### Annotating Workloads

```yaml
# billing-api-reloader.yaml (Deployment excerpt)
apiVersion: apps/v1
kind: Deployment
metadata:
  name: billing-api
  namespace: team-billing
  annotations:
    # Restart when ANY referenced Secret or ConfigMap changes
    reloader.stakater.com/auto: "true"
    # Or restart only when specific objects change
    # secret.reloader.stakater.com/reload: "billing-api-keys"
    # configmap.reloader.stakater.com/reload: "billing-feature-flags"
spec:
  replicas: 3
  selector:
    matchLabels:
      app: billing-api
  template:
    metadata:
      labels:
        app: billing-api
    spec:
      containers:
        - name: api
          image: registry.mycompany.io/billing/api:1.4.2
          envFrom:
            - secretRef:
                name: billing-api-keys
```

Prefer the explicit `secret.reloader.stakater.com/reload` form in production. `reloader.stakater.com/auto`
restarts the workload for every referenced object, including ones that change often (for example, a feature
flag ConfigMap), and frequent restarts are a rotation problem of their own.

### Controlling the Blast Radius

A restart is a rolling update, so it respects the Deployment's `maxUnavailable` and `maxSurge` settings and
any PodDisruptionBudget. Before rotating a credential that many pods use:

- Confirm that `replicas` is greater than one and that a PDB exists for the Deployment.
- Stagger rotations across workloads. Do not rotate every tenant's credentials in the same minute.
- For a Deployment that takes a long time to become ready, rotate during a low-traffic window.

---

## Pattern 4: In-Application Reload

Restarts are simple but disruptive for long-lived connections, such as streaming or WebSocket gateways. Where
the application supports it, reload in place:

| Mechanism | Example | Notes |
|-----------|---------|-------|
| File watch | Application re-reads a mounted Secret volume on change | Kubelet updates mounted Secret volumes eventually, so the app must poll or use a file watcher; environment variables never update |
| Signal | `SIGHUP` triggers a config reload | Pair with a sidecar that sends the signal when the file changes |
| Admin endpoint | `POST /admin/reload` | Platform-owned sidecar calls it after a Reloader-style event |

Environment variables set from a Secret (`envFrom`, `valueFrom`) are fixed at container start, so they always
require a restart. Mounting the Secret as a volume allows in-place reload, which is why many platforms prefer
file-based secrets for anything that rotates.

---

## Pattern 5: Certificate Rotation with cert-manager

cert-manager issues and renews X.509 certificates and writes them into Secrets. Renewal is driven by two
fields: `duration` (how long the certificate is valid) and `renewBefore` (how early to renew).

```yaml
# billing-api-tls.yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: billing-api-tls
  namespace: team-billing
spec:
  secretName: billing-api-tls
  duration: 2160h        # 90 days
  renewBefore: 360h      # renew 15 days before expiry
  commonName: api.billing.mycompany.io
  dnsNames:
    - api.billing.mycompany.io
  privateKey:
    algorithm: ECDSA
    size: 256
    rotationPolicy: Always   # rotate the key on every renewal, not only the certificate
  issuerRef:
    name: letsencrypt-prod
    kind: ClusterIssuer
    group: cert-manager.io
```

Key points:

- `rotationPolicy: Always` generates a new private key on each renewal. The default has differed across
  cert-manager releases, so set it explicitly so that a leaked key has a bounded lifetime.
- cert-manager updates the Secret in place. Consumers need the Reloader pattern or an in-app reload to pick it
  up. The Secret's `tls.crt` and `tls.key` keys stay the same, so the file path does not change.
- Monitor expiry, not just renewal. An expiry alert catches failures in the issuer or in the DNS challenge.

```promql
# Alert when any certificate expires within 7 days, regardless of why renewal failed
(certmanager_certificate_expiration_timestamp_seconds - time()) < 7 * 24 * 3600
```

---

## Pattern 6: Workload Identity Token Rotation

Projected service account tokens are rotated by the kubelet automatically. A projected volume with
`serviceAccountToken` has a `expirationSeconds` and an audience, and the kubelet refreshes the file before it
expires:

```yaml
# billing-api-projected-token.yaml (pod spec excerpt)
volumes:
  - name: vault-token
    projected:
      sources:
        - serviceAccountToken:
            audience: vault
            expirationSeconds: 3600
            path: token
containers:
  - name: api
    volumeMounts:
      - name: vault-token
        mountPath: /var/run/secrets/vault
        readOnly: true
```

The rotation burden is entirely on the reader: the application must read the token file each time it needs it,
not once at startup. Client libraries that cache the token will fail silently after the first expiry.

---

## Pattern 7: Rotating the Store's Own Credential

The hardest rotation is the one that protects the path to your secrets: the credential ESO or Vault Agent uses
to log in. If that credential is rotated carelessly, every `ExternalSecret` in the cluster stops syncing at
once.

Use **dual-credential overlap**:

```mermaid
graph TD
    A[Step 1: create credential B<br/>while A is still valid] --> B[Step 2: update store object to use B]
    B --> C[Step 3: wait for one full refresh cycle<br/>and confirm all ExternalSecrets are Ready]
    C --> D[Step 4: revoke credential A]
```

The procedure for a Kubernetes-auth ESO store does not need a stored credential at all, which is one reason
to prefer it. For a store that uses a static credential:

```bash
# 1. Create the new credential (for example, an AppRole secret ID or a cloud access key)
NEW_CRED=$(vault write -field=secret_id -f auth/approle/role/external-secrets/secret-id)

# 2. Update the Kubernetes Secret that the ClusterSecretStore references
kubectl -n external-secrets create secret generic eso-vault-approle-next \
  --from-literal=secret-id="$NEW_CRED" --dry-run=client -o yaml | kubectl apply -f -

# 3. Point the ClusterSecretStore's auth reference at the new Secret (edit it in Git so GitOps
#    applies the change), then confirm the store reports Ready
kubectl get clustersecretstore vault-backend -o jsonpath='{.status.conditions[0]}'; echo

# 4. After every ExternalSecret reports Ready, revoke the old secret ID
vault write auth/approle/role/external-secrets/secret-id/destroy secret_id="$OLD_SECRET_ID"
```

The exact auth field that names the Secret depends on the provider and auth method. Look it up in the ESO
provider guide for your store rather than copying a field path from another example.

The principle matters more than the commands: never revoke the old credential until the new one is proven,
and prove it by checking the consumers, not by checking that the new credential exists.

---

## Orchestrating a Full Rotation

A repeatable rotation for one application combines the patterns above:

| Step | Action | Automated By | Verify |
|------|--------|--------------|--------|
| 1 | Generate new value in source | Scheduled job or Vault | Source timestamp updated |
| 2 | Wait for delivery | ESO `refreshInterval`, or force-refresh annotation | `ExternalSecret` Ready, Secret resourceVersion changed |
| 3 | Restart or reload consumers | Reloader, or in-app reload | Pods replaced or reloaded; error rate unchanged |
| 4 | Confirm consumers use the new value | Application metrics or auth logs | No authentication failures with the old identity |
| 5 | Revoke the old value | Rotation job, after a grace period | Old credential rejected at source |

Keep the grace period explicit (for example, 30 minutes for a static API key) and make the job idempotent, so a
rerun after a partial failure does not create a third credential.

### Rotation Monitoring

External Secrets exposes sync metrics that make a stale secret visible before it causes an outage:

```yaml
# secrets-rotation-alerts.yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: secrets-rotation
  namespace: monitoring
  labels:
    platform.mycompany.io/owner: security-platform
spec:
  groups:
    - name: secrets-rotation
      rules:
        - alert: ExternalSecretSyncErrors
          expr: increase(externalsecret_sync_calls_error[15m]) > 0
          for: 15m
          labels:
            severity: warning
          annotations:
            summary: "ExternalSecret sync is failing; rotated values may not reach the cluster"
        - alert: ExternalSecretNotReady
          expr: externalsecret_status_condition{condition="Ready",status="False"} == 1
          for: 30m
          labels:
            severity: critical
          annotations:
            summary: "ExternalSecret has been not Ready for 30 minutes"
```

Pair these with a rule that compares the source's rotation timestamp to the Secret's last update, if your
rotation job writes one. A timestamp annotation on the target Secret makes the comparison easy to query.

---

## Policy: Making Rotation Mandatory

Rotation is easy to skip. Encode the minimum expectations as policy:

- Every `ExternalSecret` in a tenant namespace sets `refreshInterval`. Reject any that omit it.
- Dynamic credential paths (`database/creds/*`) are allowed only with a Vault role whose `max_ttl` is set.
- Workloads that consume a rotating Secret carry a Reloader annotation or an explicit exemption label.

Expressing these as admission policies is covered in the [Policy Engine](../12-security-compliance/policy-engine.md)
documentation.

---

## Hands-On Exercise

**Goal**: Rotate a static secret through ESO, have Reloader restart the consuming Deployment, and confirm the
new value is in use. Then repeat with a dynamic database credential if you have a database in the lab.

```bash
# 1. Install Reloader in the cluster
helm upgrade --install reloader stakater/reloader \
  --namespace reloader --create-namespace \
  --set reloader.watchGlobally=false

# 2. Create a namespace, a Secret, and a Deployment that opts in to reloads
kubectl create namespace rotation-lab
kubectl -n rotation-lab create secret generic api-key --from-literal=key=v1
kubectl -n rotation-lab apply -f - <<'EOF'
# rotation-lab.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: echo
  namespace: rotation-lab
  annotations:
    secret.reloader.stakater.com/reload: "api-key"
spec:
  replicas: 2
  selector:
    matchLabels:
      app: echo
  template:
    metadata:
      labels:
        app: echo
    spec:
      containers:
        - name: echo
          image: busybox:1.36
          command: ["sh", "-c", "echo key=$API_KEY && sleep 3600"]
          env:
            - name: API_KEY
              valueFrom:
                secretKeyRef:
                  name: api-key
                  key: key
EOF

# 3. Record the current pod names and the value each pod printed
kubectl -n rotation-lab get pods -l app=echo -o name > before.txt
kubectl -n rotation-lab logs deploy/echo | head -1

# 4. Rotate the value and watch the rollout
kubectl -n rotation-lab create secret generic api-key --from-literal=key=v2 \
  --dry-run=client -o yaml | kubectl -n rotation-lab apply -f -
kubectl -n rotation-lab rollout status deploy/echo
kubectl -n rotation-lab get pods -l app=echo -o name > after.txt
diff before.txt after.txt || true
kubectl -n rotation-lab logs deploy/echo | head -1
```

**Verify**:
1. `before.txt` and `after.txt` list different pod names, showing that Reloader replaced the pods.
2. The log line changes from `key=v1` to `key=v2` after the rollout.
3. `kubectl -n rotation-lab get deploy echo` shows `2/2` ready at every point during the rollout. A
   PodDisruptionBudget with `minAvailable: 1` added to the lab would not change that for a rolling restart.

**Stretch**: Replace the manual Secret with an `ExternalSecret` that reads from Vault (see
[External Secrets](./external-secrets.md)). Change the Vault value and confirm that the Secret updates within
one `refreshInterval`, and that Reloader restarts the pods as a result.

**Cleanup**: `kubectl delete namespace rotation-lab`, `helm uninstall reloader -n reloader`, and
`rm before.txt after.txt`.

---

## Troubleshooting

| Symptom | Likely Cause | Diagnose | Fix |
|---------|--------------|----------|-----|
| Secret updated in Vault, cluster still has old value | `refreshInterval` not reached, or ExternalSecret not Ready | `kubectl get externalsecret -A`; check `status.refreshTime` | Force refresh with the annotation; fix the failing condition |
| Pods did not restart after Secret change | Reloader annotation missing, or Reloader not watching the namespace | `kubectl -n reloader logs deploy/reloader`; check the annotation on the Deployment | Add the `secret.reloader.stakater.com/reload` annotation; set `watchGlobally` or add the namespace |
| Application keeps using old password after restart | Secret consumed through a volume that is cached, or a connection pool holds the old credential | Compare `kubectl exec ... env` and the file content | Restart the pods; fix the pool to re-read credentials |
| Dynamic DB credentials fail after an hour | Lease expired and the app did not reconnect | Agent logs show new lease; app logs show `password authentication failed` | Make the pool reconnect on auth errors; re-read the credential file |
| `permission denied` reading `database/creds/*` | Policy missing `read` on that path, or wrong role | `vault token capabilities <token> database/creds/billing-app` | Fix the policy; confirm the role name in the Agent annotation |
| Certificate renewed but app serves the old one | App loads the certificate once at startup | `openssl s_client -connect host:443` shows old serial | Restart via Reloader or implement file watch |
| `certmanager` renewal stuck in `Issuing` | DNS-01 or HTTP-01 challenge failing | `kubectl describe certificaterequest`, `kubectl get challenges -A` | Fix the solver and DNS records |
| All ExternalSecrets stopped syncing at once | The store credential was revoked before the new one was applied | `kubectl get clustersecretstore -o wide`; store status condition | Restore the previous credential, then perform overlap rotation correctly |
| Token file unchanged after expiration | Application cached the token value | Compare the file's mtime with the `exp` claim | Read the file on each use, or use a client library that does |

---

## Further Reading

- [External Secrets: Refresh and Reconcile](https://external-secrets.io/latest/introduction/overview/)
- [Vault Database Secrets Engine](https://developer.hashicorp.com/vault/docs/secrets/databases)
- [Vault Leases](https://developer.hashicorp.com/vault/docs/concepts/lease)
- [Reloader](https://github.com/stakater/Reloader)
- [cert-manager Certificate Resource](https://cert-manager.io/docs/usage/certificate/)
- [Kubernetes Projected Volumes](https://kubernetes.io/docs/concepts/storage/projected-volumes/)
- [Kubernetes Secrets: Mounted Secrets Are Updated Automatically](https://kubernetes.io/docs/concepts/configuration/secret/#mounted-secrets-are-updated-automatically)

---

## Next Steps

1. **Start**: [External Secrets Operator](./external-secrets.md) if you have not built the sync path yet
2. **Go dynamic**: [Vault Integration](./vault-integration.md) for Agent-based dynamic credentials
3. **Protect Git**: [Sealed Secrets](./sealed-secrets.md) for bootstrap values that must be committed
4. **Operate**: [Disaster Recovery](../11-reliability/disaster-recovery.md) for what to do when a rotation goes wrong at scale
