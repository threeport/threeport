# Kubernetes Runtimes

A Kubernetes Runtime represents a distinct Kubernetes cluster used as a
runtime environment for workloads.  You can deploy and utilize as many
Kubernetes Runtimes as your needs require.  Workloads that target a
Kubernetes Runtime are defined with Kubernetes resource manifests.

For more information about Kubernetes, see the official [Kubernetes
docs](https://kubernetes.io/docs/home/).

## Machine Runtimes

Threeport also supports Machine Runtimes for SSH-reachable hosts that sit
outside Kubernetes (VMs, bare-metal servers, edge devices).  See the
[Machine Runtimes introduction](../machine/machine-intro.md) for the
runtime model and the [Machine Workloads
introduction](../machine/machine-workload-intro.md) for the
create/update/delete script model used to manage workloads on those hosts.

## Alternative Runtimes

Kubernetes and Machine Runtimes are the runtimes built into threeport
today.  The runtime model is extensible.  New runtime types can be added
via the [threeport SDK](https://github.com/threeport/threeport/tree/main/pkg/sdk)
by defining the API types and the controller that reconciles them.

## Kubernetes Runtime Definition

The definition allows you to specify which infrastructure provider to use.
EKS on AWS, OKE on OCI and GKE on GCP are supported, as well as kind for
local development.  A Threeport control plane on OCI is not production ready,
because its database isn't backed up.  See [Install Threeport on
OCI](../install/install-threeport-oci.md).  You can also specify the node
sizes and profiles.
Currently, you can reference the [source
code](https://github.com/threeport/threeport/blob/main/pkg/mapping/v0/node.go)
to see which NodeSize and NodeProfile values are available and what machine
type each one translates to on every supported cloud provider.  All Kubernetes
Runtimes use cluster autoscaling and you can specify the maximum number of
nodes to allow in the cluster.

Reference:
[KubernetesRuntimeDefinition](https://pkg.go.dev/github.com/threeport/threeport/pkg/api/v0#KubernetesRuntimeDefinition)

## KubernetesRuntimeInstance

This represents a deployed instance of a Kubernetes cluster.  You can specify
which location you would like to use.  Currently, you can reference the [source
code](https://github.com/threeport/threeport/blob/main/pkg/mapping/v0/location.go)
for the available Location values and which cloud provider regions they
correspond to.

### Network Ranges

Each EKS, GKE and OKE cluster gets its own network range, a `/16`, recorded on
its kubernetes runtime instance as `NetworkCidr`, so no two clusters of one
control plane overlap and their networks can be connected. The control plane
allocates the first free `/16` of its pool when the cluster is provisioned. A
range set on the instance when it's created is used instead: a `/16` or larger
that no other runtime has and no child control plane's pool holds, which
doesn't have to be in the pool.

The pool is the control plane's `NetworkCidrPool`. By default it is
`10.16.0.0/16` through `10.255.0.0/16`, except `10.96.0.0/16` and
`10.244.0.0/16`, which OKE uses inside each cluster. No range may hold those,
`172.20.0.0/16`, where EKS services get addresses, or `172.16.0.0/28`, the GKE
control plane's private endpoint. It starts at `10.16`, clear
of `10.0.0.0/12`, where clusters created before ranges were allocated have
their networks, so new clusters can be connected to them. An organization that
already uses part of `10.0.0.0/8` can give the control plane its own pool.

Every range of a cluster is laid out in its block:

| Provider | Ranges |
|---|---|
| EKS | the VPC is the block; a `/18` private subnet per zone at `.0.0`, `.64.0` and `.128.0`, for nodes and pods; a `/22` public subnet per zone at `.192.0`, `.196.0` and `.200.0`; services in `172.20.0.0/16` |
| GKE | pods in the `/17` at `.0.0`, a `/25` for each of 256 nodes, which run up to 64 pods each; nodes in the `/20` at `.128.0`; services in the `/20` at `.144.0` |
| OKE | the VCN is the block; the API endpoint, node and load balancer subnets where they have always been, relative to it; pods and services in `10.244.0.0/16` and `10.96.0.0/16` |

`tptctl up` takes the control plane's pool with `--network-cidr-pool`, and its
own cluster's range with `--network-cidr`, by default the pool's first block.
With `--control-plane-only`, `--network-cidr` records the existing cluster's
range, so no runtime is allocated it.

A child control plane allocates its clusters' ranges from the
`NetworkCidrPool` its parent gives it on the child's control plane instance.
Without one, it allocates none, and its clusters get the network every cluster
used to get, since the default pool is its parent's. The parent doesn't
allocate from a child's pool, a child's pool can't overlap another child's or
a range the parent has allocated, and it can only grow: a block can be added,
or replaced by a larger one that holds it, but not removed, since the child may
have allocated from it.

The clusters a child allocated from its pool live in the child's database, and
may outlive the child. So when a child with a pool is deleted, its pool is
recorded in `ReservedNetworkCidrs` on the control plane instance that
represents its parent, and the parent doesn't allocate from it or accept a
runtime range or child pool that overlaps it. Once the clusters allocated from
a reserved block are gone, release it by removing it from
`ReservedNetworkCidrs`: get the control plane instance with `tptctl get
control-plane-instance -o yaml`, remove the block, and apply the file with
`tptctl replace control-plane-instance`. A config file that leaves
`ReservedNetworkCidrs` out keeps the reservations as they are.

A cluster created before ranges were allocated keeps its network, and records
no range. So does one whose create is resumed on a stack that holds its network
already: it keeps the range that network was laid out in. Separate Threeport
installations don't know each other's allocations: give their control planes
pools that don't overlap if their clusters are to be connected.

Reference:
[KubernetesRuntimeInstance](https://pkg.go.dev/github.com/threeport/threeport/pkg/api/v0#KubernetesRuntimeInstance)

## Next Steps

We have a [Remote Kubernetes Runtime](remote-kubernetes-runtime.md) guide that
walks you through the creation of a Kubernetes cluster to use for your workloads
in Threeport.

