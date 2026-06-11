# Install Threeport on Oracle Cloud Infrastructure

This guide provides instructions to install Threeport on
[Oracle Kubernetes Engine](https://www.oracle.com/cloud-native/container-engine-kubernetes/)
(OKE). We will spin up a new OKE cluster and install the Threeport core system
there. It requires an Oracle Cloud account, API keys (or another supported auth
method for the OCI CLI), and permission to create the needed resources. This
install method is useful for testing Threeport on a remote cloud provider.

If you would prefer to test out Threeport locally, see our guide to [Install
Threeport Locally](install-threeport-local.md).

Note: this guide requires you to have our tptctl command-line tool installed. See
our [Install tptctl guide](install-tptctl.md) to install if you haven't already.

## Install Threeport

> Requirement: You need to have the [Pulumi CLI](https://www.pulumi.com/docs/install/)
> installed locally when using `tptctl` to install Threeport on OCI.

This section assumes you have an Oracle Cloud Infrastructure tenancy and the
[OCI CLI](https://docs.oracle.com/en-us/iaas/Content/API/Concepts/cliconcepts.htm)
configured on your machine. Follow Oracle’s documentation to create a user,
generate an API key, and populate `~/.oci/config` with a profile (the default
profile name is often `DEFAULT`).

If `~/.oci/config` exists and you can run `oci iam region list` successfully,
you are likely already set up.

Ensure your user has permission to create networking, Kubernetes clusters, IAM
resources, and related objects in the tenancy (or compartment, depending on your
policies). For exploratory setups, membership in the `Administrators` group or
equivalent broad access is often enough; tighten permissions for production use.

You need an [OCI region
identifier](https://docs.oracle.com/en-us/iaas/Content/General/Concepts/regions.htm)
where OKE will run (for example `us-phoenix-1`).

With credentials configured, run the following to install Threeport on OKE:

```bash
tptctl up \
  --name test \
  --provider oke \
  --oci-region [oci region]  # e.g. us-phoenix-1
```

The `oke` provider **requires** `--oci-region`. The `tptctl up` command marks
this flag as required when `--provider oke` is used.

By default, Threeport reads OCI credentials from the profile named `DEFAULT` in
`~/.oci/config`. To use another profile, pass:

```bash
  --oci-config-profile [profile name]
```

This process will usually take on the order of 15–25 minutes. You will see
output as Oracle Cloud resources are created. It provisions an OKE cluster and
installs the core system components. It also registers that cluster as the default
Kubernetes runtime for tenant workloads.

Other `tptctl up` flags you may use include `--debug`, `--teardown-on-failure`,
and `--force-overwrite-config`. Run `tptctl up --help` for the full list.

## Validate Deployment

Note: if you would like to use
[kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl)
against the cluster where Threeport is running, and you have the OCI CLI
installed, merge kubeconfig credentials for the cluster. The cluster’s display
name is `threeport-{instance-name}` — for `--name test`, that is `threeport-test`.

Obtain the cluster **OCID** from the Oracle Cloud Console (Developer Services →
Kubernetes Clusters → your cluster), then run:

```bash
oci ce cluster create-kubeconfig \
  --cluster-id [cluster ocid] \
  --file $HOME/.kube/config \
  --region [oci region] \
  --token-version 2.0.0
```

Append `--kube-endpoint PUBLIC_ENDPOINT` if you need the public Kubernetes API
endpoint (common for remote access).

Then, view the Threeport core system pods with kubectl:

```bash
kubectl get pods -n threeport-control-plane
```

## Next Steps

Next, we suggest you deploy a sample workload with Threeport to get a clear idea
of dependency management capabilities. See our
[Deploy Workload Locally guide](../workloads/deploy-workload-local.md) for
instructions you can follow against your OKE-backed control plane.

## Clean Up

If you're done for now and not installing a workload, you can uninstall
Threeport:

```bash
tptctl down --name test
```
