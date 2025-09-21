# Install Threeport on OCI

This guide provides instructions to install Threeport on
[Oracle Container Engine for Kubernetes (OKE)](https://docs.oracle.com/en-us/iaas/Content/ContEng/Concepts/contengoverview.htm).  We will spin up
a new OKE cluster and install the Threeport core system there.  It requires you
have an OCI account and API keys.  This install method is useful for testing
Threeport on a remote cloud provider.

If you would prefer to test out Threeport locally, see our guide to [Install
Threeport Locally](install-threeport-local.md)

Note: this guide requires you have our tptctl command line tool installed.  See
our [Install tptctl guide](install-tptctl.md) to install if you haven't already.

**Before proceeding, ensure you have the required OCI IAM permissions configured.**
See our [OCI IAM Permissions guide](../oci/oci-iam.md) to set up the necessary permissions
for your OCI user.

## Install Pulumi

The Threeport OCI provider uses Pulumi to deploy necessary infrastructure. See the
[Pulumi documentation](https://www.pulumi.com/docs/iac/download-install/) for your
system's appropriate installation steps.

## Install Threeport

This section assumes you already have an OCI account and credentials configured on
your local machine.  Follow the OCI
[CLI setup guide](https://docs.oracle.com/en-us/iaas/Content/API/SDKDocs/cliinstall.htm)
for steps on how to do this.

Note: if you have the `~/.oci/config` file on your
file system, you're likely already set up.

Also, ensure you have the required permissions to create the necessary resources
in OCI. Refer to our [OCI IAM Permissions guide](../oci/oci-iam.md) referenced above
for detailed instructions on setting up the necessary permissions.

You also will need your OCI tenancy OCID.  It can be found in the OCI console.
Log in to OCI and look at the top-right of the console.  Click on your profile
and select "Tenancy: [your-tenancy-name]".  The OCID will be displayed there.

With credentials configured, run the following to install Threeport in OKE:

```bash
tptctl up \
    --name test \
    --provider oke \
    --oci-region [oci region]  # e.g. us-ashburn-1
```

This process will usually take 10-15 minutes.  It can take even longer on some
OCI accounts.  You will see output as OCI resources are created. It will create a remote
OKE Kubernetes cluster and install all of the core system components.  It will also
register the same OKE cluster as the default Kubernetes cluster
for tenant workloads.

## Validate Deployment

Note: if you would like to use
[kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl)
against the cluster where Threeport is
running, and you have the [OCI CLI](https://docs.oracle.com/en-us/iaas/Content/API/SDKDocs/cliinstall.htm)
installed, you can update your kubeconfig
with a command given in the OCI cloud console on the "Clusters" dashboard
located [here](https://cloud.oracle.com/containers/clusters)

Then, view the Threeport core system pods with kubectl:

```bash
kubectl get pods -n threeport-control-plane
```

## Next Steps

Next, we suggest you deploy a sample workload to OCI using Threeport.  It will
give you clear idea of Threeport's dependency management capabilities.

Note: A dedicated OCI workload deployment guide is coming soon. For now, you can adapt the
[Deploy Workload on AWS guide](../workloads/deploy-workload-aws.md) principles for OCI usage.

## Clean Up

If you're done for now and not installing a workload on OCI, you can
uninstall Threeport:

```bash
tptctl down --name test
```