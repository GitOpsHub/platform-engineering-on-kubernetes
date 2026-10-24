# Runbook: Rollback Procedures

Four rollback paths, each with a different reach. Know which one you need before you start:

| Path | Undoes | Use it when |
|------|--------|-------------|
| (a) Helm rollback | One Helm release's manifests and values | A chart upgrade broke a platform add-on |
| (b) Argo CD rollback (Git revert) | Whatever Git and Argo CD manage | A change in the GitOps repo caused the problem |
| (c) Control-plane limits | Nothing; this section explains what you cannot undo | A Kubernetes upgrade went wrong |
| (d) Velero | Lost or damaged API objects in a namespace | You deleted or corrupted resources and need them back |

---

## (a) Helm rollback

Helm keeps a revision history per release. Each `helm upgrade` creates a new revision, and `helm rollback` creates another revision that points at old content.

```bash
helm history <release> -n <ns>
helm rollback <release> <revision> -n <ns> --wait
```

Replace `<release>` and `<ns>` with the release name and namespace, and `<revision>` with a number from
`helm history`. Expected result: `Rollback was a success! Happy Helming!`. Check with `helm history`, which
shows a new top revision described `Rollback to <revision>`.

Example, rolling back Traefik on blue to revision 3:

```bash
helm --kube-context kind-platform-engineering history traefik -n traefik
helm --kube-context kind-platform-engineering rollback traefik 3 -n traefik --wait
```

Expected result: the history lists at least three revisions before you roll back. If it does not, pick a
revision that exists.

Notes:
- `--wait` returns only when the release's resources are ready. Without it, the command returns before pods are healthy.
- Helm rollback restores the chart and values from that revision. It does not restore CRDs. Check whether the chart's CRDs changed before you roll back an add-on that ships CRDs (cert-manager, for example).
- If Argo CD manages the release, a Helm rollback is overwritten on the next sync. Use path (b) for GitOps-managed releases.

---

## (b) Argo CD rollback

### Git revert (the GitOps path)

For anything Argo CD manages, the source of truth is Git. The rollback is a commit that reverts the bad change:

```bash
git -C <platform-config-clone> revert <bad-commit-sha>
git -C <platform-config-clone> push origin main
argocd app sync <app-name>
argocd app wait <app-name> --health --timeout 600
```

Replace `<platform-config-clone>` with the path of your local clone of the GitOps repo, `<bad-commit-sha>` with
the commit to undo, and `<app-name>` with the affected Argo CD application. Expected result: `git revert`
creates a new commit, and `argocd app wait` returns with `Health Status: Healthy`. Check with
`argocd app get <app-name>`, which shows `Sync Status: Synced` at the new revision.

Argo CD syncs the reverted state and the cluster converges back to it. This is the rollback to use by default, because the history stays in Git and the next sync will not reintroduce the bad change.

If auto-sync is on, Argo CD will apply the revert on its own. The `argocd app sync` line is only needed when you want to force it.

### UI history rollback

Argo CD's UI keeps a history of previous synced revisions for each application. In the UI, open the application, choose **History and Rollback**, and roll back to a previous revision.

Use this for a fast emergency rollback. After it, fix the Git source, or the next sync from Git will put the bad revision back. With auto-sync on, the UI rollback is reverted by the sync, so turn auto-sync off first.

---

## (c) Kubernetes control-plane rollback limits

**You cannot downgrade the control plane.** Once `kube-apiserver` and `etcd` have moved to a new minor, there is no supported in-place downgrade:

- The API server and etcd may have written data in the new format or with new fields, and older binaries cannot read or convert it back.
- `kubeadm` does not provide a supported downgrade of the control plane.
- Restoring an old etcd snapshot rolls the whole cluster state back to that moment, including every change made since, on every node. It is a disaster recovery action, not a rollback.

What you can do:

1. **Cut over to blue.** If blue still exists and is healthy, the safe rollback is the ingress switch back (blue-green-deploy.md, step 5b). This is why blue is kept until the soak is complete.
2. **Restore from backup.** Restore an etcd snapshot (for a real cluster) or a cluster rebuild from your infrastructure code and GitOps repo, then re-apply workloads. Velero does not restore etcd or the control plane; it restores API objects in namespaces (section d).
3. **Recreate the cluster** from the pinned previous version, then restore workloads from Git and from Velero.

So the order to protect yourself is: keep the old cluster until the new one is proven, and have a backup of the old cluster's objects before you touch the control plane.

---

## (d) Velero

Velero backs up Kubernetes API objects and, with volume support, persistent volume data. It stores backups in an S3-compatible bucket or a cloud object store. It can restore into the same cluster or a different one.

### Object store choice

Velero needs an object store. Options on a laptop:

- **A local S3-compatible service** on the `kind` Docker network. Blue's nodes can reach it by container name, and the lab's data stays on your machine. This is the default below.
- **A real S3 bucket** in an AWS account. Velero's AWS plugin works unchanged, and the configuration is fully verified. Use this if the local option does not start.

The default below uses a local SeaweedFS container. **Its exact flags are not verified for this lab.** Check them against your version before you rely on it:

```bash
docker run --rm chrislusf/seaweedfs:4.49 server -h
```

Start the store on the `kind` network so blue's nodes can resolve it by name:

```bash
docker run -d --name velero-s3 --network kind -p 8333:8333 chrislusf/seaweedfs:4.49 server -s3 -dir=/data
```

Create a bucket and a credential pair for Velero. Use the credential method your SeaweedFS version documents. Then check that the bucket exists:

```bash
aws --endpoint-url http://localhost:8333 s3 mb s3://velero
aws --endpoint-url http://localhost:8333 s3 ls
```

Expected result: `make_bucket: velero`, then a listing that includes `velero`. The `aws` CLI needs
credentials set in your environment (`AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`) that match the pair you
create for SeaweedFS. Replace `<access-key>` and `<secret-key>` in the next block with that same pair.

### Install Velero into blue

Install the Velero CLI (v1.18.x). Velero's own install step puts the server into the cluster, using the current kubectl context:

```bash
kubectl config use-context kind-platform-engineering

cat > velero-credentials <<'EOF'
[default]
aws_access_key_id=<access-key>
aws_secret_access_key=<secret-key>
EOF

velero install \
  --provider aws \
  --plugins velero/velero-plugin-for-aws:v1.14.4 \
  --bucket velero \
  --secret-file ./velero-credentials \
  --backup-location-config region=us-east-1,s3ForcePathStyle=true,s3Url=http://velero-s3:8333 \
  --use-volume-snapshots=false \
  --wait

rm -f velero-credentials
velero backup-location get       # should show Available
```

Expected result: the location `default` shows `Available`. If it shows `Unavailable`, check the bucket name,
credentials and that `velero-s3` is on the `kind` network (`docker network inspect kind`).

Notes:
- `--use-volume-snapshots=false` is required here: kind's default `local-path` StorageClass has no CSI snapshot support, so no volume snapshot class exists to use. The lab backs up API objects only. **Persistent volume data is not in these backups.** Do not treat a restore from them as a full data restore.
- The `velero-s3` name is the container name on the `kind` network. Use `http://velero-s3:8333` from inside the cluster, not `localhost`.
- The plugin version is paired with the Velero server version. v1.14.4 is unverified against the 1.18 server; if `velero install` reports a plugin incompatibility, check the plugin's compatibility table.

### Back up

Back up the smoke-test namespace before any risky change:

```bash
velero backup create hello-pre-upgrade \
  --include-namespaces platform-smoke-test \
  --snapshot-volumes=false \
  --wait

velero backup describe hello-pre-upgrade --details
velero backup get
```

Expected result: the backup `hello-pre-upgrade` has status `Completed` with 0 errors.

`describe --details` lists every resource in the backup. Check that the Deployment, Service, Ingress and Certificate are present before you trust it.

### Restore into a scratch namespace

Restore into a different namespace so you do not collide with the live `hello` app. `--namespace-mappings` takes `source:destination`:

```bash
velero restore create hello-restore-test \
  --from-backup hello-pre-upgrade \
  --namespace-mappings platform-smoke-test:platform-smoke-test-restore \
  --wait

velero restore describe hello-restore-test --details
kubectl --context kind-platform-engineering -n platform-smoke-test-restore get deploy,svc,ingress
```

Expected result: the restore shows `Phase: Completed`, and the scratch namespace holds the `hello`
Deployment, Service and Ingress. The restored Ingress uses the same host name as the live one, so do not
test through the front door while it exists.

Clean up the scratch namespace when you are done:

```bash
kubectl --context kind-platform-engineering delete namespace platform-smoke-test-restore
```

Expected result: `namespace "platform-smoke-test-restore" deleted`.

### Restore to a real rollback

To roll back a deleted or corrupted namespace in place, restore into the original namespace name. Delete the damaged objects first, or Velero skips existing ones.

> **Warning:** Restoring into the original namespace changes live state. Do not run it against a
> namespace that is still in use without a second backup in hand.

```bash
velero restore create <restore-name> --from-backup hello-pre-upgrade --wait
```

Replace `<restore-name>` with a unique name. Expected result: `Phase: Completed` in
`velero restore describe <restore-name>`.

### Before you decommission blue

Take a final backup of blue's workloads, and keep it in the bucket until green has been running for the agreed period. It is the record of what blue served, not a way to bring blue back: the cluster itself is gone.
