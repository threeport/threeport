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

### High Availability and Node Counts

`HighAvailability` decides how many zones a cluster runs in:

| Provider | `HighAvailability: true` | `HighAvailability: false` |
|---|---|---|
| EKS | nodes in 3 zones | nodes in 2 zones |
| GKE | a regional cluster: its control plane replicated across the region's zones, and its nodes in 3 of them | a zonal cluster: a single control plane replica, and its nodes in one zone |

On GKE the zones are the region's first, by name, that are up. A cluster stays
in the zones it was created in, since moving it to others replaces it.

`NodeMaximum` limits the number of nodes in the whole cluster, across all its
zones, on both EKS and GKE. It is 60 when a definition doesn't set it. It was
250 before, and a definition created then keeps its own. It must be at least
2, the number of nodes a cluster starts with, and at least 3 for a highly
available GKE cluster, whose zones need a node each.

GKE creates a node pool's initial nodes in each of its zones, so the initial
count is divided across the zones and rounded up: a highly available GKE
cluster starts with one node in each of its three zones. A GKE definition
whose initial count, rounded up that way, is more than its maximum is
refused.

On EKS and GKE, `NodeSize`, `NodeProfile` and `NodeMaximum` are copied into the
EKS or GKE definition when it's created, and can't be changed on the
kubernetes runtime definition afterwards. Change the node group's sizes on the
EKS or GKE definition instead.

Set them, with `NodeSize` and `NodeProfile`, in a config file:

```yaml
KubernetesRuntimeDefinition:
  Name: production
  InfraProvider: gke
  HighAvailability: true
  NodeSize: Large
  NodeProfile: Balanced
  NodeMaximum: 30
```

### Kubernetes Version

On EKS, GKE and OKE, `KubernetesVersion` sets the Kubernetes minor version the
clusters are created on, such as `1.36`. The supported versions are `1.35` and
`1.36`, and `1.36` is the default. Each is offered by all three providers: in
standard support on EKS, in GKE's `REGULAR` and `STABLE` release channels, and
as a production release on OKE.

- **EKS** creates the cluster and its node group on that version. The cluster
  autoscaler's release matches it.
- **GKE** creates the control plane on the release its channel offers for that
  version, and the nodes on the control plane's. GKE upgrades both past it on
  its own.
- **OKE** creates the cluster and its nodes on the newest release of that
  version that OKE offers and has a worker node image for the node shape.

The version is recorded on the EKS, GKE or OKE definition, and it can't be
changed once the definition exists: Threeport doesn't upgrade a cluster yet. A
kind cluster's version comes with its node image, so it takes none.

`tptctl up` sets the version of the cluster it creates with
`--kubernetes-version`.

### Upgrades

Each provider upgrades a cluster on its own in some cases. Threeport sets
how:

- **GKE** enrolls the cluster in a release channel, `REGULAR` by default, and
  upgrades its control plane and nodes to that channel's releases within a
  weekly maintenance window, apart from an upgrade GKE can't defer. The window
  defaults to Saturday and Sunday from 04:00 to 12:00 UTC. A GKE definition
  sets them with `ReleaseChannel` (`REGULAR` or `STABLE`),
  `MaintenanceWindowDays` (such as `SA,SU`), `MaintenanceWindowStart` (a UTC
  time of day, such as `04:00`) and `MaintenanceWindowHours` (4 to 24). The
  cluster of a production control plane that `tptctl up` creates is on
  `STABLE`. Like the version, they can't be changed once the definition
  exists.
- **EKS** upgrades the control plane to the next version when its version
  leaves standard support, rather than keeping it on extended support at a
  higher hourly charge. The node group isn't upgraded with it.

Reference:
[KubernetesRuntimeDefinition](https://pkg.go.dev/github.com/threeport/threeport/pkg/api/v0#KubernetesRuntimeDefinition)

## KubernetesRuntimeInstance

This represents a deployed instance of a Kubernetes cluster.  You can specify
which location you would like to use.  Currently, you can reference the [source
code](https://github.com/threeport/threeport/blob/main/pkg/mapping/v0/location.go)
for the available Location values and which cloud provider regions they
correspond to.

Reference:
[KubernetesRuntimeInstance](https://pkg.go.dev/github.com/threeport/threeport/pkg/api/v0#KubernetesRuntimeInstance)

## Next Steps

We have a [Remote Kubernetes Runtime](remote-kubernetes-runtime.md) guide that
walks you through the creation of a Kubernetes cluster to use for your workloads
in Threeport.

