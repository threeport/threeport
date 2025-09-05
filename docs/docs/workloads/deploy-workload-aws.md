# Deploy Workload on AWS

In this guide, we're going to deploy a sample WordPress app and use Threeport to
manage several dependencies for it:

* Network ingress routing
* TLS termination
* DNS record using AWS Route53
* Managed database using AWS RDS
* Managed object storage using AWS S3
* Managed secrets using AWS Secrets Manager

## Prerequisites

You'll need a Threeport control plane for this guide.  You have two options:

1. Install a [Local Threeport](../install/install-threeport-local.md) instance and
   then provision a [Remote Kubernetes
   Runtime](../kubernetes-runtime/remote-kubernetes-runtime.md) for your workload.
1. Install a [Remote Threeport](../install/install-threeport-aws.md) instance
   on AWS and use the Kubernetes instance that is used to host Threeport to deploy
   your workload.

> TODO
