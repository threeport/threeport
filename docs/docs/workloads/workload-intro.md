# Workloads

A Workload is a set of Kubernetes resources that constitute some software that
you want to run on a server somewhere.

## Workload vs Application

What do we mean by these terms?  A "workload" is just an arbitrary
grouping of Kubernetes resources.  To be meaningful, it generally implies that
it includes Kubernetes resources that run some kind of containerized software,
such as:

* Pod
* Deployment
* StatefulSet
* DaemonSet
* Job

An application is a software system that provides value to an end user.  That
software system could be a single workload or a group of workloads in a
distributed system.  Many applications include not just workloads, but also managed
services offered by a cloud provider, such as a managed database or job queue.

## Workload Constitution

This begs the question of how you should define and separate Workloads that make
up some distributed application.  There are no hard and fast rules.  It is up to
the Threeport user but here are some considerations:

* If a code repo produces a single program that runs on a server and serves
  traffic, such as a web application, that would make sense to deploy as a
  Workload.
* Anything that can be updated or upgraded independently of other services in a
  distributed application system, should be managed as its own distinct
  Workload.
* A web application that uses a dedicated database that always has a one-to-one
  relationship, both the web app and database can be deployed together as a
  single Workload, since one without the other is meaningless.
* If different components of a software system are managed by different teams,
  it usually works best for each team to define and use distinct Workloads in
  Threeport.

## Workload Definition

The definition for a workload includes all the Kubernetes resources needed to
run the containerized workload.  You will need to create a Kubernetes resource
manifest to reference in the Workload Definition config.

Reference:
[WorkloadDefinition](https://pkg.go.dev/github.com/threeport/threeport/pkg/api/v0#WorkloadDefinition)

## Workload Instance

A workload instance allows you to specify which Kubernetes Runtime Instance you
would like to run the workload in.

Reference:
[WorkloadInstance](https://pkg.go.dev/github.com/threeport/threeport/pkg/api/v0#WorkloadInstance)

> Note: You can also run multiple instances of a workload in a single Kubernetes
> cluster if you use Threeport to manage Kubernetes namespaces.  See the
> [Namespaces guide](namespaces.md) for more info.

## Next Steps

In order to get a practical grasp on deploying Workloads, see our [Local
Workload guide](deploy-workload-local.md) to try it out on your workstation.

If you'd like to deploy a sample Workload into AWS using Threeport, see our
[Remote Workload guide](deploy-workload-aws.md).

