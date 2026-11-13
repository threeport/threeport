# Install Threeport on GCP

This guide provides instructions to install Threeport on
[Google Kubernetes Engine](https://cloud.google.com/kubernetes-engine/). We will
spin up a new GKE cluster and install the Threeport core system there. It
requires a Google Cloud project and credentials with permission to create the
needed resources. This install method is useful for testing Threeport on a remote
cloud provider.

If you would prefer to test out Threeport locally, see our guide to [Install
Threeport Locally](install-threeport-local.md).

Note: this guide requires you to have our tptctl command-line tool installed. See
our [Install tptctl guide](install-tptctl.md) to install if you haven't already.

Note: The Kubernetes Engine API must be enabled in your GCP project.

## Install Threeport

> Requirement: You need to have the [Pulumi CLI](https://www.pulumi.com/docs/install/)
> installed locally when using `tptctl` to install Threeport on GCP.

This section assumes you have a Google Cloud project and credentials configured
on your local machine. Follow Google Cloud’s
[authentication overview](https://cloud.google.com/docs/authentication)
to sign in (for example with `gcloud auth application-default login`) and set a
default project if you use `gcloud` defaults (`gcloud config set project
PROJECT_ID`).

`tptctl` acts on the GKE cluster as you, with these same application default
credentials, for as long as you manage the control plane with it - including
`tptctl down`. The control plane's own controllers never use them: on GKE they
authenticate as their own Workload Identity.

You need:

- **Project ID** — the Google Cloud project ID (not necessarily the display
  name) where Threeport will provision infrastructure.
- **Region** — the
  [Google Cloud region](https://cloud.google.com/compute/docs/regions-zones)
  where the GKE cluster will run (for example `us-central1`).

Ensure your principal has sufficient permissions to create GKE clusters,
networking, IAM service accounts, and related resources in that project. For
exploratory setups, a role such as `roles/owner` or `roles/editor` on the project
is often enough; tighten permissions for production use.

With credentials and project details ready, run the following to install
Threeport on GKE:

```bash
tptctl up \
  --name test \
  --provider gke \
  --gcp-project-id [gcp project id] \
  --gcp-region [gcp region]  # e.g. us-central1
```

The `gke` provider **requires** both `--gcp-project-id` and `--gcp-region`.
Those values override project and region taken from environment variables when
set (see `tptctl up --help` for details).

This process will usually take on the order of 10–20 minutes. You will see
output as Google Cloud resources are created. It provisions a regional GKE
cluster and installs the core system components. It also registers that cluster
as the default Kubernetes runtime for tenant workloads.

Other `tptctl up` flags you may use include `--debug`, `--teardown-on-failure`,
and `--force-overwrite-config`. Run `tptctl up --help` for the full list.

A production control plane, which you ask for with `--tier production`, backs
its database up to a Cloud Storage bucket in another region. Choose that region with
`--gcp-backup-region`. See
[Back Up and Restore a Control Plane](../control-planes/backup-restore.md).

### Production Cluster

A production control plane runs its database as three replicas, and its
cluster is built to keep serving after the loss of one zone:

| | Development | Production |
|---|---|---|
| Zones | 3 (regional cluster) | 3 (regional cluster) |
| Machine type | `e2-medium`: shared-core, 1 vCPU and 4 GB, bursting above its baseline | `e2-standard-4`: 4 vCPU and 16 GB, no bursting |
| Nodes per zone | 2 to start, 1 to 10 | 1 to start, 1 to 10 |
| Placing new nodes | in any zone with capacity (`ANY`) | keeping the zones even (`BALANCED`) |
| Database replicas | 1 | 3, each in its own zone and on its own node |

A production cluster therefore starts with three `e2-standard-4` nodes, one
per zone, where a development cluster starts with six `e2-medium` nodes. See
[Compute Engine pricing](https://cloud.google.com/compute/vm-instance-pricing)
for what each costs in your region. A database replica that can't be placed
in a zone of its own stays `Pending`, rather than sharing a zone it would be
lost with.

The control plane's kubernetes runtime definition records the cluster:
`HighAvailability`, and the `NodeSize` and `NodeProfile` its machine type maps
from (`Large` and `Balanced` for production, `Medium` and `Balanced` for
development).

## Restrict Access to the Kubernetes API

By default the cluster's Kubernetes API accepts connections from any address.
Requests still need credentials. To accept connections only from networks you
trust, pass `--k8s-api-allowed-cidrs` with a comma-separated list of IPv4 CIDR
blocks, up to 50:

```bash
  --k8s-api-allowed-cidrs 203.0.113.7/32,198.51.100.0/24
```

Include the public address of the machine you run `tptctl` from. `tptctl`
installs the control plane through the Kubernetes API, and `tptctl down` and
`tptctl upgrade` use it as well. Once the cluster exists, `tptctl up` checks
that it can reach the Kubernetes API before it installs anything. If it can't
within two minutes, it stops with an error that names the endpoint, and deletes
the cluster it created. Run `tptctl up` again with your address in the list.
To keep the cluster after the error instead, pass
`--teardown-on-failure=false`, and delete it later with
`tptctl down --name test --infra-only`.

Addresses that belong to Google Cloud, such as other projects' VMs or Cloud
Shell, are not let through unless they are in the list.

The control plane reaches the cluster it runs on from inside the cluster, so
it needs no entry in the list.

## Validate Deployment

Note: if you would like to use
[kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl)
against the cluster where Threeport is running, and you have the
[`gcloud` CLI](https://cloud.google.com/sdk/gcloud) installed, you can fetch
cluster credentials with:

```bash
gcloud container clusters get-credentials threeport-test \
  --region [gcp region] \
  --project [gcp project id]
```

Use the same `--name` you passed to `tptctl up` in place of `test` in the
cluster name `threeport-test`.

Then, view the Threeport core system pods with kubectl:

```bash
kubectl get pods -n threeport-control-plane
```

## Next Steps

Next, we suggest you deploy a sample workload with Threeport to get a clear idea
of dependency management capabilities. See our
[Deploy Workload Locally guide](../workloads/deploy-workload-local.md) for
instructions you can follow against your GKE-backed control plane.

## Clean Up

If you're done for now and not installing a workload, you can uninstall
Threeport:

```bash
tptctl down --name test
```

The database backup bucket of a production control plane is kept. Add
`--delete-backups` to delete it with the control plane.

A production control plane's cluster is protected from deletion, so `tptctl
down` refuses to tear it down until you turn the protection off. See
[Deletion Protection](../kubernetes-runtime/kubernetes-intro.md#deletion-protection).
