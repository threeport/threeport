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

## What the install provisions

When `tptctl up --provider gke` installs a production control plane, it
creates:

- **A backup bucket** named `<runtime>-backups-<project>`, for example
  `threeport-prod-backups-acme-prod`. It is private, with uniform bucket-level
  access and public access prevention enforced, and labelled as belonging to
  the control plane. Its name is recorded in your Threeport config.
- **A backup service account**, `threeport-bkp-<runtime>`, that holds only
  `roles/storage.objectCreator` and `roles/storage.objectViewer` on the bucket.
  It can write and read backups, but neither delete one nor change the bucket.
- **A Workload Identity binding** that lets the database's Kubernetes service
  account, `crdb` in the `threeport-control-plane` namespace, act as the backup
  service account. The database authenticates to the bucket without a key.

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

1. scales the API server and controllers to zero, so nothing writes to the
   database while it is replaced
2. restores the backup alongside the database, under `threeport_api_restore`
3. drops the database and renames the restored one into its place, handing
   its tables back to the user the API server connects as
4. takes a full backup of the restored database, so the hourly incremental
   backups keep working on top of it
5. scales the control plane back up and waits for the API server to be ready

If the restore fails before step 3, the database is left as it was and the
control plane is scaled back up on it. The restore runs as the Job
`threeport-restore-database`, which is left in place when it fails so its logs
can be read:

```bash
kubectl logs -n threeport-control-plane job/threeport-restore-database
```

Running the restore again finishes one that failed part way.

To restore from a bucket other than the one in your Threeport config, for
example a copy of the backups, pass it with `--bucket`. The database's backup
service account needs `roles/storage.objectViewer` on that bucket.

### After a restore

The restored database holds the API objects as they were when the backup was
taken. Objects created since are gone from the API, but whatever they created,
such as cloud infrastructure or workloads in other clusters, is still running
and is no longer managed by Threeport. Objects deleted since are back in the
API, while what they managed may already be gone.

Review what changed between the backup and the restore and reconcile it, by
recreating the objects that were lost or removing what they left behind.

## Tearing a control plane down

`tptctl down` removes the backup service account and its access to the bucket,
but keeps the bucket and every backup in it, since they may still be needed
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

## Control planes installed before backups

A production control plane installed before backups existed has no bucket and
no schedule, and `tptctl reinstall` does not add them, since it leaves the
database and its stateful set in place. Install the control plane again to get
backups.
