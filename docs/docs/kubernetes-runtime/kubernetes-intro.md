# Kubernetes Runtimes

A Kubernetes Runtime is currently the only Threeport-supported runtime environment
for workloads.  Each instance represents a distinct Kubernetes cluster.  You can
deploy and utilize as many Kubernetes Runtimes as your needs required.

As such Workloads in Threeport require Kubernetes resource manifests to deploy
them.

For more information about Kubernetes, see the official [Kubernetes
docs](https://kubernetes.io/docs/home/).

## Alternative Runtimes

It is possible to add support for alternative runtime environments such as machines, i.e.
deploying directly to a server using a machine image.

Alternative runtimes are not on the Threeport roadmap but could be incorporated.

## Kubernetes Runtime Definition

The definition allows you to specify which infrastructure provider to use
(currently only EKS on AWS is supported).  You can also specify the node sizes
and profiles.  Currently, you can reference the [source
code](https://github.com/threeport/threeport/blob/main/internal/kubernetes-runtime/mapping/node.go)
to see which NodeSize and NodeProfile values are available and what AWS machine
types these translate to.  All Kubernetes Runtimes use cluster autoscaling and
you can specify the maximum number of nodes to allow in the cluster.

Reference:
[KubernetesRuntimeDefinition](https://pkg.go.dev/github.com/threeport/threeport/pkg/api/v0#KubernetesRuntimeDefinition)

## KubernetesRuntimeInstance

This represents a deployed instance of a Kubernetes cluster.  You can specify
which location you would like to use.  Currently, you can reference the [source
code](https://github.com/threeport/threeport/blob/main/internal/kubernetes-runtime/mapping/location.go)
for the available Location values and which AWS regions they correspond
to.

Reference:
[KubernetesRuntimeInstance](https://pkg.go.dev/github.com/threeport/threeport/pkg/api/v0#KubernetesRuntimeInstance)

## Next Steps

We have a [Remote Kubernetes Runtime](remote-kubernetes-runtime.md) guide that
walks you through the creation of a Kubernetes cluster to use for your workloads
in Threeport.

