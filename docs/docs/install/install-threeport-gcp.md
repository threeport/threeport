# Install Threeport on GCP

This guide provides instructions to install Threeport on
[Google Kubernetes Engine](https://cloud.google.com/kubernetes-engine/). We will
spin up a new GKE cluster and install the Threeport core system there. It
requires a Google Cloud project and credentials with permission to create the
needed resources. This install method is useful for testing Threeport on a remote
cloud provider.

If you would prefer to test out Threeport locally, see our guide to [Install
Threeport Locally](install-threeport-local.md).

Note: this guide requires you have our tptctl command line tool installed. See
our [Install tptctl guide](install-tptctl.md) to install if you haven't already.

Note: The Kubernetes Engine API must be enabled in your GCP project.

## Install Threeport

This section assumes you have a Google Cloud project and credentials configured
on your local machine. Follow Google Cloud’s
[authentication overview](https://cloud.google.com/docs/authentication)
to sign in (for example with `gcloud auth application-default login`) and set a
default project if you use `gcloud` defaults (`gcloud config set project
PROJECT_ID`).

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
