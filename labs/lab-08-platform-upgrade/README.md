# Lab 08: Platform Upgrade - Blue-Green and Canary

## Objectives
- Stand up a **green** cluster on the next Kubernetes minor (v1.36) next to the **blue** cluster from Lab 01 (v1.35)
- Install Lab 01's add-ons on green, then use Argo CD in blue to deploy Lab 01's platform baseline and smoke app to green
- Cut traffic from blue to green at the ingress, smoke-test it, and keep blue as the rollback target
- Roll out one platform add-on as a canary before promoting it
- Run the pre-flight checks an in-place minor upgrade needs (deprecated APIs, PDBs, node health, skew)
- Back up with Velero, restore into a scratch namespace, and learn which rollbacks are possible and which are not

## Prerequisites

| Tool | Version | Notes |
|------|---------|-------|
| Lab 01 | completed | The blue cluster `kind-platform-engineering` must be running |
| Lab 02 | completed | Argo CD runs in blue (see [Lab 02](../lab-02-gitops-bootstrap/README.md)) |
| Docker | 25+, 12 GB RAM recommended | Two kind clusters at once; Lab 01 alone needs 8 GB |
| kind | 0.33.0+ | Same binary as Lab 01 |
| kubectl | 1.35 or 1.36 | Must be within one minor of both clusters |
| Helm | 3.12+ | Installs Cilium, Traefik and cert-manager on green |
| argocd CLI | v3.5.x | Required here (`argocd cluster add`) |
| velero CLI | v1.18.x | Required for the backup step |
| aws CLI | v2 | Creates the Velero bucket in the local S3-compatible store (or use a real S3 bucket) |
| kubent | 0.7.x | Optional. Deprecated-API check, preferred over pluto |
| pluto | 5.x | Optional. Used if kubent is not installed |

## Estimated Time

2-3 hours. About 30 minutes of that is image pulls and two cluster bootstraps. Velero and the canary add 45-60 minutes.
Difficulty: Advanced.

---

## Files in this lab

| File | Purpose |
|------|---------|
| [`green-kind-cluster.yaml`](green-kind-cluster.yaml) | Green kind cluster on v1.36, host ports 9080/9443 |
| [`blue-green-deploy.md`](blue-green-deploy.md) | Create, register, cut over, decommission |
| [`canary-upgrade.md`](canary-upgrade.md) | Canary add-on rollout, minor upgrade ordering, skew, deprecated APIs |
| [`rollback-procedures.md`](rollback-procedures.md) | Helm, Argo CD, control-plane limits, Velero |
| [`upgrade-automation.sh`](upgrade-automation.sh) | Read-only pre-flight checks; `--execute` gates every cordon/drain |
| [`teardown.sh`](teardown.sh) | Deletes green, the Velero store and the smoke-test leftovers; leaves blue running |

### Pinned versions

| Component | Version | Source |
|-----------|---------|--------|
| Kubernetes blue | v1.35.8 | Lab 01 `kind-cluster.yaml` |
| Kubernetes green | v1.36.4 (`kindest/node`) | `green-kind-cluster.yaml` |
| Cilium | 1.20.4 | Lab 01 `setup.sh` (installed on green the same way) |
| Argo CD | v3.5.6 (chart 10.11.1) | Lab 02 |
| Velero CLI | v1.18.x | Velero 1.18 line |
| Velero server | 1.18.2 (chart 12.2.1) | |
| Velero AWS plugin | v1.14.4 | Used for the S3-compatible backend |
| kubent | 0.7.3 | kube-no-trouble |
| pluto | v5.24.4 | FairwindsOps |

Versions were checked against public release metadata on 2026-10-24. Items marked "unverified" in the runbooks (the SeaweedFS store and the Velero plugin pairing) need checking before you rely on them.

## What this lab can and cannot model

A laptop has one Docker daemon, one host IP and one host port 8080. A real platform upgrade depends on multi-region control planes, managed-provider upgrade APIs, load balancers that shift weighted traffic, and DNS. This lab models the **shape** of the process: two independent clusters, one GitOps controller in blue that also deploys to green, a traffic cutover, and a rollback path.

Be clear about what is simulated:
- Both clusters run on one machine, so there is no real failure domain.
- Blue's Traefik owns host port 8080, so **blue stays the single edge**. The cutover is real at the ingress layer (it changes which backend serves the hostname), but the edge itself is not redundant.
- Argo CD lives in blue. In production you would run a controller per failure domain, or a controller that is itself outside the clusters it manages.

## Design

```text
            laptop
  127.0.0.1:8080 (blue)          127.0.0.1:9080 (green)
          |                               |
+---------v-------------------+   +-------v-----------------------+
| blue: platform-engineering  |   | green: platform-engineering-  |
| Kubernetes v1.35.8 (Lab 01) |   | green, Kubernetes v1.36.4     |
|                             |   |                               |
| Argo CD (Lab 02) ---------------->  registered as a cluster     |
| Traefik                     |   |                               |
|   hello.localtest.me        |   |  Traefik  hello.localtest.me  |
|     -> hello (blue)         |   |  -> hello (green)             |
|     -> hello-green-proxy ------->  NodePort 30080 on green      |
+-----------------------------+   +-------------------------------+
```

- **Blue** is the cluster from Lab 01, unchanged. Argo CD from Lab 02 runs here.
- **Green** is a new kind cluster from [`green-kind-cluster.yaml`](green-kind-cluster.yaml) on the next minor. Argo CD does not run on green; blue's controller deploys to it.
- **Traffic** reaches green through a blue-side Service whose Endpoints point at green's Traefik NodePort on the shared `kind` Docker network. The switch is a one-line change to the Ingress backend (a `kubectl patch` here, because blue's Ingress comes from Lab 01's `setup.sh`; a Git commit in production).

Runbooks:
- [Blue-green deploy](blue-green-deploy.md): create green, register it, cut over, decommission blue
- [Canary upgrade](canary-upgrade.md): roll out one add-on first, then the Kubernetes minor upgrade ordering and the pre-flight checks
- [Rollback procedures](rollback-procedures.md): Helm, Argo CD, control plane limits, Velero

Background reading:
[Disaster Recovery](../../docs/11-reliability/disaster-recovery.md),
[Argo CD Deep Dive](../../docs/05-gitops/argocd-deep-dive.md),
[Multi-Cluster GitOps](../../docs/05-gitops/multi-cluster-gitops.md),
[Kubernetes Architecture](../../docs/01-kubernetes-fundamentals/kubernetes-architecture.md),
[Drift Detection](../../docs/04-infrastructure-as-code/drift-detection.md).

---

## Steps

Each step links to a runbook section that gives the command, the expected result and a check. Run them in
this order from `labs/lab-08-platform-upgrade`.

1. **Create green**: [blue-green-deploy.md, steps 1-2](blue-green-deploy.md#step-1-create-the-green-cluster)
2. **Register green with blue's Argo CD**: [step 3](blue-green-deploy.md#step-3-register-green-with-argo-cd-in-blue)
3. **Install the add-ons on green and deploy the baseline and smoke app**: [step 4](blue-green-deploy.md#step-4-install-the-add-ons-on-green-and-deploy-the-baseline)
4. **Canary one add-on on green**: [canary-upgrade.md, part 1](canary-upgrade.md#part-1-canary-add-on-rollout)
5. **Run the pre-flight checks** (dry run): `./upgrade-automation.sh --target-version 1.36`
6. **Back up blue with Velero and test a restore**: [rollback-procedures.md, section d](rollback-procedures.md#d-velero)
7. **Cut traffic over to green and smoke test**: [blue-green-deploy.md, step 5](blue-green-deploy.md#step-5-switch-traffic)
8. **Decommission blue**, or keep it running as your rollback target: [blue-green-deploy.md, step 7](blue-green-deploy.md#step-7-decommission-blue)

What to expect after each step:

| Step | Expected result |
|------|-----------------|
| 1 | A second kind cluster, `platform-engineering-green`, with 3 Ready nodes on v1.36.4 |
| 2 | `argocd cluster list` shows `platform-engineering-green` connected |
| 3 | Traefik and cert-manager are deployed on green, `argocd app list` shows `platform-addons-platform-engineering-green` Synced and Healthy, and `platform-ca` is Ready on green |
| 4 | The canary app is Healthy on green only, and the test Certificate becomes Ready |
| 5 | A summary line `Summary: PASS=<n> WARN=<n> FAIL=<n> SKIP=<n>`, and the planned steps (nothing is changed) |
| 6 | `velero backup get` shows `hello-pre-upgrade` as `Completed`, and the restore into `platform-smoke-test-restore` completes |
| 7 | `curl -s http://hello.localtest.me:8080/hostname` prints a green pod name |
| 8 | Only if you decommission: `kind get clusters` lists only `platform-engineering-green` |

---

## Verification

```bash
kubectl --context kind-platform-engineering version -o json | grep gitVersion   # blue: v1.35.8
kubectl --context kind-platform-engineering-green version -o json | grep gitVersion   # green: v1.36.4
kubectl --context kind-platform-engineering-green get nodes                       # 3 Ready
argocd cluster list                                                               # both clusters listed
argocd app list                                                                   # platform-addons-platform-engineering-green Synced / Healthy

# Direct smoke test of green (bypasses the switch):
curl -s -H 'Host: hello.localtest.me' http://127.0.0.1:9080/hostname

# Through the front door (blue's ingress, now switched to green):
curl -s http://hello.localtest.me:8080/hostname   # prints a pod name from green

./upgrade-automation.sh --target-version 1.36      # dry run; read the summary
```

Expected result: the two `gitVersion` lines show `v1.35.8` and `v1.36.4` (each command also prints a client
version line), the green cluster lists 3 `Ready` nodes, and both `curl` commands print a green pod name. The
last command prints a `Summary:` line with `FAIL=0`.

Done when the switched hostname returns a pod from green, Argo CD shows the green application Healthy, and a Velero restore into `platform-smoke-test-restore` succeeds.

---

## Cleanup

> **Warning:** This deletes the green cluster and the local Velero store, including every backup in it.

Revert the traffic switch first (see [rollback of the switch](blue-green-deploy.md#rollback-of-the-switch)).
Otherwise blue's Ingress still points at `hello-green-proxy`, and `teardown.sh` removes that Service and the
green cluster behind it.

```bash
./teardown.sh
```

`teardown.sh` does the following, and each part is safe to repeat:

- Deletes the Service `hello-green-proxy` in `platform-smoke-test` on blue.
- Deletes the namespace `platform-smoke-test-restore` on blue.
- Deletes the kind cluster `platform-engineering-green`.
- Removes the `velero-s3` Docker container (the local S3-compatible store).

Expected result: `==> Done. Blue cluster left in place; ...`. Blue keeps running. The script does not remove
the Argo CD objects, the Velero server, or any background port-forward. Remove those yourself if you no longer
need them:

```bash
kubectl --context kind-platform-engineering -n argocd delete applicationset platform-addons --ignore-not-found
argocd cluster rm platform-engineering-green
velero uninstall            # uses the current kubectl context: switch to blue first
pkill -f 'port-forward svc/argocd-server 18080'
rm -f applicationset.yaml cert-manager-canary.yaml   # files the runbooks had you create
```

Check with `kind get clusters` (only `platform-engineering` remains) and `docker ps --filter name=velero-s3`
(no rows).

Blue is deleted by Lab 01's `teardown.sh`, and only after you have moved Argo CD off it (see the
[decommission step](blue-green-deploy.md#step-7-decommission-blue)). Leave blue running if you want to keep
Lab 01 or do the next lab.

---

## Troubleshooting

### `argocd cluster add` fails or the cluster shows `Unknown`
`cluster add` runs on your laptop, so it needs the **host** address (`127.0.0.1:<random port>`). Argo CD's controller runs inside blue's nodes and needs the **Docker-network** name. Step 3 registers with the host address first, then patches the stored server to `https://platform-engineering-green-control-plane:6443`. If you skipped that patch, the cluster stays `Unknown` or `Failed`.

### Blue cannot reach green's API server
From a blue node, the green control-plane name should resolve:
```bash
docker exec platform-engineering-control-plane getent hosts platform-engineering-green-control-plane
```
If it does not, check that both kind clusters are on the `kind` Docker network: `docker network inspect kind`.

### Port 8080 is already in use when you start `port-forward`
Lab 01's Traefik publishes host port 8080, so a `kubectl port-forward ... 8080:443` fails. Use `18080` (as in step 3) and `argocd login localhost:18080`.

### Switched traffic still shows blue
Check the backend with the step 5b check command. If it still prints `hello`, run the 5b patch again. If you
put blue's Ingress under an Argo CD application (for example by adding blue to the ApplicationSet), `selfHeal`
reverts the patch: make the change in Git instead, or take blue out of that application.

### Green node IP changed
The Endpoints object stores an IP. If the green control-plane container restarts with a new address, update it:
```bash
docker inspect -f '{{ (index .NetworkSettings.Networks "kind").IPAddress }}' platform-engineering-green-control-plane
```

### `upgrade-automation.sh` prints SKIP for deprecated APIs
Neither kubent nor pluto is in your PATH. This is a SKIP, not a pass. Install one and re-run.

---

## Next Steps

1. **Read**: [Disaster Recovery](../../docs/11-reliability/disaster-recovery.md) for RTO/RPO framing
2. **Read**: [Multi-Cluster GitOps](../../docs/05-gitops/multi-cluster-gitops.md) for how a real fleet handles cluster registration
3. **Revisit Lab 06** (observability) and put the canary metrics from [canary-upgrade.md](canary-upgrade.md#metrics-to-watch) on a dashboard

---

## Resources

- [Kubernetes version skew policy](https://kubernetes.io/releases/version-skew-policy/)
- [kubeadm upgrade guide](https://kubernetes.io/docs/tasks/administer-cluster/kubeadm/kubeadm-upgrade/)
- [Argo CD cluster management](https://argo-cd.readthedocs.io/en/stable/operator-manual/declarative-setup/#clusters)
- [Argo CD ApplicationSet cluster generator](https://argo-cd.readthedocs.io/en/stable/operator-manual/applicationset/Generators-Cluster/)
- [Velero documentation](https://velero.io/docs/)
- [kube-no-trouble (kubent)](https://github.com/doitintl/kube-no-trouble)
- [Pluto](https://github.com/FairwindsOps/pluto)
- [kind documentation](https://kind.sigs.k8s.io/)
