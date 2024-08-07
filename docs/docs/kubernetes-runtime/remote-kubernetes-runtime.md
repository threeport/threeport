# Remote Kubernetes Runtime

Threeport supports the management of Kubernetes clusters as remote runtimes.
This pattern may be used to run workloads on separate Kubernetes clusters from the one
that the Threeport API is deployed to.

## Prerequisites

An instance of the Threeport is required to get started.  You can install a
[Local Threeport](../install/install-threeport-local.md) instance and use it to
create a remote Kubernetes runtime.

Note that [AWS EKS](https://aws.amazon.com/eks/) clusters are currently the only
supported type of remote Kubernetes runtime.

## AWS Account Setup

First, create a work space on your local file system:

```bash
mkdir threeport-runtime-test
cd threeport-runtime-test
```

To get started, a valid `AwsAccount` object must be created. Use the [Basic AWS Setup guide](../aws/basic-aws-setup.md) for instructions.

## Deployment

Kubernetes clusters are represented as `KubernetesRuntime` objects in the Threeport API.

Use the following command to download a sample Kubernetes Runtime config:

```bash
curl -O https://raw.githubusercontent.com/threeport/releases/main/samples/k8s-runtime.yaml
```

If you open the file it will look as follows:

```yaml
KubernetesRuntime:
  Name: eks-remote
  InfraProvider: eks
  InfraProviderAccountName: default-account
  HighAvailability: false
  Location: NorthAmerica:NewYork
  DefaultRuntime: true
```

The `Name` field is an arbitrary name for the user to assign.

The `InfraProvider` indicates we will use AWS EKS to spin up the Kubernetes
cluster.

The `InfraProviderAccountName` references the name of the AWS account we
created above.

The `HighAvailability` field determines the number of availability zones (AZs) the
cluster will be installed across.  When `false` it will installed across two AZs.

The `Location` field tells Threeport where to install the Kubernetes cluster.
`NorthAmerica:NewYork` is a Threeport abstraction that allows users to reference
a common set of locations, regardless of provider.  For AWS, this translates to
the `us-east-1` region.  When other cloud providers are supported, it will
reference the appropriate region for the cloud provider being used.  For now,
you can reference the [Threeport source
code](https://github.com/threeport/threeport/blob/main/internal/kubernetes-runtime/mapping/location.go#L49)
to see which locations map to which regions in AWS.

The `DefaultRuntime` field indicates that, when deploying workloads, if a
Kubernetes Runtime is not specified, it will use this one by default.

Create a `KubernetesRuntime` instance:
```bash
tptctl create kubernetes-runtime --config k8s-runtime.yaml
```

View the status of the deployed Kubernetes runtime instance:
```bash
tptctl get kubernetes-runtime-instances
```

Note: if you would like to use
[kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl)
against the cluster where Threeport is
running and you have the [AWS CLI](https://aws.amazon.com/cli/)
installed you can update your kubeconfig
with:

```bash
aws eks update-kubeconfig --name threeport-test
```

## Cleanup


Run the following command to delete the remote Kubernetes runtime instance:
```bash
tptctl delete kubernetes-runtime-instance --name eks-remote
```

Clean up the downloaded config files:
```bash
rm aws-account.yaml
rm k8s-runtime.yaml
```
