# AWS

Amazon Web Services (AWS) is a natively supported cloud provider on Threeport.

Kubernetes runtime environments can be managed on AWS with core Threeport resources.
This is implemented using the Elastic Kubernetes Service (EKS).

There are many other AWS resources that are managed by Threeport to deliver these
EKS clusters.  VPCs, subnets, elastic load balancers, etc. are all managed in
service of the runtime environment, but Threeport users need not configure or
provision these separately.

## AWS Account

An AWS Account object allows you to register AWS account information with
Threeport so that it can be used to deploy runtimes, workloads and managed
services in that account.  A
[genesis](../../control-planes/control-plane-intro#control-plane-instance)
Threeport control plane deployed to AWS will utilize AWS best-practice
[IRSA](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html)
authentication to manage resources within its own AWS account.  To give
Threeport access to other AWS accounts, you must include an AWS account ID,
access key ID, and secret access key credentials to authenticate. If you have
your local AWS config set up to use the `aws` CLI tool you can reference those
credentials stored on your local file system when creating an external AWS
account.

You can register and use as many AWS accounts in Threeport as you wish.

Reference:
[AwsAccount](https://pkg.go.dev/github.com/threeport/threeport/pkg/api/v0#AwsAccount)

## AWS EKS Kubernetes Runtime Definition

This object allows you to configure an AWS EKS cluster directly.  We recommend
using the `KubernetesRuntimeDefinition` object with the `InfraProvider` field
set to `eks` to provision EKS clusters.  However, if there is a specific EC2
instance type that you'd like to use that isn't offered through the Threeport
NodeProfile and NodeSize abstractions, you can directly provision EKS clusters
using this object.

When you create one of these objects, Threeport will create a corresponding
Kubernetes Runtime Definition so that it can be referenced by the system as
needed.

Reference:
[AwsEksKubernetesRuntimeDefinition](https://pkg.go.dev/github.com/threeport/threeport/pkg/api/v0#AwsEksKubernetesRuntimeDefinition)

## AWS EKS Kubernetes Runtime Instance

This object allows you to provision an instance from the config in a definition.
Similar to the definition, we recommend using the `KubernetesRuntimeInstance` to
provision EKS clusters in AWS.  However, if you need to specify a region not
offered through the Threeport Location abstraction, you can use this object.

When you create one of these objects, Threeport will create a corresponding
Kubernetes Runtime Instance so that it can be referenced by the system as
needed.  This Kubernetes Runtime Instance contains the connection information
for the Kubernetes API that is used by the workload controller to deploy
resources.

Reference:
[AwsEksKubernetesRuntimeInstance](https://pkg.go.dev/github.com/threeport/threeport/pkg/api/v0#AwsEksKubernetesRuntimeInstance)

## Next Steps

Check out our [Deploy Workload on AWS guide](../workloads/deploy-workload-aws.md)
for an example of how to deploy a workload on AWS.
