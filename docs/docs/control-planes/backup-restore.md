# Back Up and Restore a Control Plane

Every Threeport API object lives in the control plane's CockroachDB database.
A production control plane on GKE backs that database up to a Google Cloud
Storage bucket in another region, so the backups survive the loss of the
cluster's region, and `tptctl restore` replaces the database with one of them.

Backups protect against corruption, accidental deletion and the loss of a
region. They complement, rather than replace, running the database on more
than one node, which protects against the loss of a node.

Backups are provisioned for production control planes whose GKE cluster
`tptctl up` creates. Development control planes, control planes installed on an
existing cluster with `--control-plane-only`, and control planes on other
providers have none.

Backups cover the `threeport_api` database, which holds every Threeport API
object. The databases of installed modules are not included.

## What the install provisions

When `tptctl up --provider gke` installs a production control plane, it
creates:

- **A backup bucket** named `<runtime>-backups-<project>`, for example
  `threeport-prod-backups-acme-prod`. It is private, with uniform bucket-level
  access and public access prevention enforced, and labelled as belonging to
  the control plane. Its name is recorded in your Threeport config.
- **Access for the database, and only the database.** The bucket grants
  `roles/storage.objectCreator` and `roles/storage.objectViewer` to the
  database's Kubernetes service account, `crdb` in the `threeport-control-plane`
  namespace, through Workload Identity Federation for GKE. It can write and
  read backups, but neither delete one nor change the bucket, and it
  authenticates without a key or a GCP service account.

The grant names the Kubernetes service account by the UID its cluster gave
it, not by its namespace and name. Every GKE cluster in a project shares one
workload identity pool, so a grant by namespace and name would also let in any
other cluster in the project that creates a `threeport-control-plane`
namespace with a `crdb` service account. A UID cannot be reproduced in
another cluster. The service account is kept across a reinstall so its UID,
and with it the database's access, survives.

The bucket goes in another region on the same continent as the cluster. To
choose the region, pass `--gcp-backup-region`:

```bash
tptctl up \
  --name prod \
  --provider gke \
  --gcp-project-id acme-prod \
  --gcp-region us-central1 \
  --gcp-backup-region us-east1
```

A backup region that is the cluster's own region is refused. For a cluster
region without a default pairing, `tptctl up` asks for `--gcp-backup-region`.

## The backup schedule

Before the API server starts, the database migrator creates a CockroachDB
backup schedule named `threeport_api_backup`:

- a full backup of the `threeport_api` database every day
- an incremental backup every hour, on top of the latest full one
- the first full backup as soon as the schedule exists, typically within a
  few minutes of the install

Backups are written under `threeport_api/` in the bucket, one directory per
full backup, named for when it was taken:

```bash
gcloud storage ls gs://threeport-prod-backups-acme-prod/threeport_api/2026/10/
```

```
gs://threeport-prod-backups-acme-prod/threeport_api/2026/10/20-050726.51/
gs://threeport-prod-backups-acme-prod/threeport_api/2026/10/21-000000.00/
```

To check the schedule and its last runs, open a SQL shell in the database pod:

```bash
kubectl exec -it -n threeport-control-plane crdb-0 -- \
  /cockroach/cockroach sql --certs-dir=/cockroach/cockroach-certs \
  -e "SHOW SCHEDULES" \
  -e "SELECT job_id, status, created, error FROM [SHOW JOBS] WHERE job_type = 'BACKUP' ORDER BY created DESC LIMIT 5"
```

## Backups before migrations

Before the API server starts, its database migrator applies any pending schema
migrations. When there are some, a production control plane first takes a full
backup into the same bucket, so an upgrade that goes wrong can be undone. A
migration that fails is rolled back automatically, and only when that rollback
fails too is the database left between versions. The migrator's error then
names the backup it took, which `tptctl restore --backup` restores. See the
database migrator's README for how a failed migration is reported and cleared.

## Restore the database

`tptctl restore` replaces the control plane's database with a backup. It
reaches the GKE cluster with your own Google credentials rather than through
the Threeport API, so it works when the API is down. Your principal needs
access to the cluster, as it does for `tptctl up`.

Everything written to the control plane since the backup was taken is lost,
so the command requires `--confirm` with the control plane's name. To restore
the latest backup:

```bash
tptctl restore --name prod --confirm prod
```

To restore a particular backup, pass its path as listed in the bucket, the
part after `threeport_api`:

```bash
tptctl restore --name prod --confirm prod --backup /2026/10/20-050726.51
```

A path restores that full backup together with the incremental backups taken
on top of it.

The restore:

1. scales the API server and controllers to zero, and waits until none of
   their pods is left, including one still running its database migrator or
   shutting down, so nothing writes to the database while it is replaced
2. **prepare:** restores the backup alongside the database, under
   `threeport_api_restore`
3. **swap:** drops the database and renames the restored one into its place,
   handing its tables back to the user the API server connects as
4. **backup:** takes a full backup of the restored database, so the hourly
   incremental backups keep working on top of it
5. scales the control plane back up and waits for the API server to be ready

Each of steps 2 to 4 runs as its own Job, `threeport-restore-database-prepare`,
`-swap` and `-backup`. A Job that fails is left in place so its logs can be
read:

```bash
kubectl logs -n threeport-control-plane job/threeport-restore-database-prepare
```

The control plane comes back only on a database that is whole:

- **Prepare fails:** the database was not touched, so the control plane is
  scaled back up on it.
- **Swap fails:** the database may be part way between the live and the
  restored one, so the control plane is left scaled down. The swap is never
  retried on its own; run the restore again with the same backup. It starts
  from prepare, restoring the backup afresh, finishes the swap wherever the
  last one stopped, and brings the control plane back to the replica counts it
  had. Until a restore completes, one that fails before its own swap also
  leaves the control plane down, since the earlier swap may have left the
  database part way.
- **Backup fails:** the database is restored and the control plane comes back,
  but the scheduled incremental backups fail until the next scheduled full one.

Only one restore of a control plane runs at a time: a restore holds the
`threeport-restore-lock` ConfigMap while it runs, and another started meanwhile
stops without changing anything. If a restore was interrupted so that it could
not release the lock, the next one says so; once you have checked that no
restore is running, delete the ConfigMap and run the restore again.

Before it changes anything, a restore stops any Job an earlier attempt left
behind, such as one whose `tptctl` exited during the swap, so two attempts
never change the database together. A Job that does not finish in time is
deleted before anything else happens. If
it cannot be confirmed gone, the control plane is left scaled down, since the
Job may still be changing the database.

To restore from a bucket other than the one in your Threeport config, for
example a copy of the backups, pass it with `--bucket`. The database's
Kubernetes service account needs `roles/storage.objectViewer` on that bucket.
The full backup the restore ends with still goes to the bucket in your
Threeport config, where the scheduled backups go.

### After a restore

The restored database holds the API objects as they were when the backup was
taken. Objects created since are gone from the API, but whatever they created,
such as cloud infrastructure or workloads in other clusters, is still running
and is no longer managed by Threeport. Objects deleted since are back in the
API, while what they managed may already be gone.

Review what changed between the backup and the restore and reconcile it, by
recreating the objects that were lost or removing what they left behind.

## Tearing a control plane down

`tptctl down` removes the database's access to the bucket, but keeps the
bucket and every backup in it, since they may still be needed
after the control plane is gone. A control plane installed again under the same
name and project reuses the bucket and the backups already in it.

To delete the bucket and its backups along with the control plane:

```bash
tptctl down --name prod --delete-backups
```

To delete a kept bucket later:

```bash
gcloud storage rm --recursive gs://threeport-prod-backups-acme-prod
```
