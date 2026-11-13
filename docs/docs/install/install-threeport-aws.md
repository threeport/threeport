# Install Threeport on AWS

This guide provides instructions to install Threeport on
[AWS Elastic Kubernetes Service](https://aws.amazon.com/eks/).  We will spin up
a new EKS cluster and install the Threeport core system there.  It requires you
have an AWS account and API keys.  This install method is useful for testing
Threeport on a remote cloud provider.

If you would prefer to test out Threeport locally, see our guide to [Install
Threeport Locally](install-threeport-local.md)

Note: this guide requires you have our tptctl command line tool installed.  See
our [Install tptctl guide](install-tptctl.md) to install if you haven't already.

## Install Threeport

> Requirement: You need to have the [Pulumi CLI](https://www.pulumi.com/docs/install/)
> installed locally when using `tptctl` to install Threeport on AWS.

This section assumes you already have an AWS account and credentials configured on
your local machine with a profile named "default".  Follow the AWS
[quickstart page](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-quickstart.html)
for steps on how to do this.

Note: if you have the `~/.aws/config` and `~/.aws/credentials` files on your
file system, you're likely already set up.

Also, ensure you have the required permissions to create the necessary resources
in AWS.  If your user has the built-in `AdministratorAccess` policy attached, you can
continue.  Otherwise, check out our [AWS Permissions guide](../aws/aws-iam.md)
to make sure you can create the resources required to run a Threeport core system.

You also will need your AWS account ID.  It can be found in the AWS console.
Log in to AWS and look at the top-right of the console.  It will say something like
`username @ 1111-2222-3333`.  The 12 digit number (without dashes) is your account ID.

With credentials configured, run the following to install Threeport in EKS:

```bash
tptctl up \
    --name test \
    --provider eks \
    --aws-region [aws region]  # e.g. us-east-1
```

This process will usually take 10-15 minutes.  It can take even longer on some
AWS accounts.  You will see output as AWS resources are created. It will create a remote
EKS Kubernetes cluster and install all of the core system components.  It will also
register the same EKS cluster as the default Kubernetes cluster
for tenant workloads.

## Restrict Access to the Kubernetes API

By default the cluster's Kubernetes API accepts connections from any address.
Requests still need credentials. To accept connections only from networks you
trust, pass `--k8s-api-allowed-cidrs` with a comma-separated list of IPv4 CIDR
blocks, up to 40:

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

The control plane reaches the cluster it runs on from inside the cluster, so
it needs no entry in the list.

## Validate Deployment

Note: if you would like to use
[kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl)
against the cluster where Threeport is
running, and you have the [AWS CLI](https://aws.amazon.com/cli/)
installed, you can update your kubeconfig
with:

```bash
aws eks update-kubeconfig --name threeport-test --region [aws region]
```

Then, view the Threeport core system pods with kubectl:

```bash
kubectl get pods -n threeport-control-plane
```

## Next Steps

Next, we suggest you deploy a sample workload to AWS using Threeport.  It will
give you clear idea of Threeport's dependency management capabilities.  See our
[Deploy Workload on AWS guide](../workloads/deploy-workload-aws.md) for instructions.

## Clean Up

If you're done for now and not installing a workload on AWS, you can
uninstall Threeport:

```bash
tptctl down --name test
```

A production control plane's cluster is protected from deletion, so `tptctl
down` refuses to tear it down until you turn the protection off. See
[Deletion Protection](../kubernetes-runtime/kubernetes-intro.md#deletion-protection).
