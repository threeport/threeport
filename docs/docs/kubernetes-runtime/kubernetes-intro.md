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

### Kubernetes Version

On EKS, GKE and OKE, `KubernetesVersion` sets the Kubernetes minor version the
clusters are created on, such as `1.36`. The supported versions are `1.35` and
`1.36`, and `1.36` is the default. Each is offered by all three providers: in
standard support on EKS, in GKE's `REGULAR` and `STABLE` release channels, and
as a production release on OKE.

- **EKS** creates the cluster and its node group on that version. Its
  `vpc-cni`, `coredns` and `kube-proxy` run as EKS add-ons, on the versions EKS
  gives for it, rather than the copies EKS installs with a cluster, which
  nothing upgrades. The cluster autoscaler's release matches it.
- **GKE** creates the control plane on the release its channel offers for that
  version, and the nodes on the control plane's. GKE upgrades both past it on
  its own.
- **OKE** creates the cluster and its nodes on the newest release of that
  version that OKE offers and has a worker node image for the node shape.

The version is recorded on the EKS, GKE or OKE definition. A kind cluster's
version comes with its node image, so it takes none.

`tptctl up` sets the version of the cluster it creates with
`--kubernetes-version`.

### Deletion Protection

With `DeletionProtection: true`, the clusters of a definition on EKS, GKE or
OKE are protected from being deleted by mistake:

| Provider | Protected against deletion through |
|---|---|
| EKS | Threeport, Pulumi, and EKS itself: the AWS console, CLI and API refuse to delete the cluster. |
| GKE | Threeport and Pulumi. GKE has no deletion protection of its own, so the cluster can still be deleted in the Google Cloud console or with `gcloud`. |
| OKE | Threeport and Pulumi. OKE has no deletion protection of its own, so the cluster can still be deleted in the OCI console or with the OCI CLI. |

- Threeport refuses to delete a runtime instance whose cluster is protected,
  answering `409`, and `tptctl down` refuses to tear down a control plane
  whose cluster is protected. Neither deletes anything. `tptctl down
  --control-plane-only` leaves the cluster as it is, so it isn't refused.
- Pulumi protects the cluster and its node group or node pool, so a change
  that would destroy or replace them fails before it changes anything.

`tptctl up --tier production` protects the cluster it creates. Otherwise it
is off by default, so development clusters stay quick to tear down.

The kubernetes runtime definition asks for it when the clusters are created,
and it is copied into the EKS, GKE or OKE definition. That definition's
`DeletionProtection` is the one that counts afterwards: the kubernetes runtime
definition can't be changed once it exists.

To delete a protected cluster, turn its protection off on the EKS, GKE or OKE
definition first, here for EKS:

```bash
tptctl get aws-eks-kubernetes-runtime-definition --name <definition> -o yaml
```

Write the definition's fields to a config file, with `DeletionProtection:
false`:

```yaml
AwsEksKubernetesRuntimeDefinition:
  Name: <definition>
  ZoneCount: 3
  DefaultNodeGroupInstanceType: t3.xlarge
  DefaultNodeGroupInitialSize: 3
  DefaultNodeGroupMinimumSize: 3
  DefaultNodeGroupMaximumSize: 10
  DeletionProtection: false
```

Then replace the definition with it, and delete the runtime instance or run
`tptctl down`:

```bash
tptctl replace aws-eks-kubernetes-runtime-definition --config definition.yaml --name <definition>
```

Use `gcp-gke-kubernetes-runtime-definition` or
`oci-oke-kubernetes-runtime-definition` on GKE or OKE. A replace that leaves
`DeletionProtection` out keeps it as it is.

Turning protection off on a definition lets every runtime instance of it be
deleted. Threeport removes the cluster's protection just before it destroys
the cluster, without applying any other change to it.

A cluster is protected when it is created, so protection can't be turned on
for a definition that already has runtime instances: their clusters would
stay unprotected while the definition said otherwise. Create the clusters to
protect from a definition with `DeletionProtection: true`.

`tptctl down --infra-only` tears down a cluster whose control plane may no
longer be running. It checks `DeletionProtection` when the Threeport API
answers, and refuses when the API answers with an error. When the API can't
be reached, or doesn't answer within 30 seconds, it warns that it skipped the
check and tears the cluster down.

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
  `STABLE`. They can't be changed once the definition exists.
- **EKS** upgrades the control plane to the next version when its version
  leaves standard support, rather than keeping it on extended support at a
  higher hourly charge. The node group isn't upgraded with it.

### Upgrading a Cluster

On EKS and GKE, raising `KubernetesVersion` on an `AwsEksKubernetesRuntime` or
`GcpGkeKubernetesRuntime` definition upgrades its clusters to that version. Set
it in the config file and replace the definition, or update the definition
through the API. The new version must be a supported one, no older than the one
it replaces: a cluster can't be downgraded. The kubernetes runtime definition
keeps the version it was created with. OKE clusters can't be upgraded yet.

The controller upgrades each cluster one minor version at a time. Each step:

1. Reads the version the cluster runs from the provider, so an upgrade the
   provider made on its own is recognized, and a cluster newer than its
   definition isn't downgraded.
2. Refreshes the cluster's Pulumi stack against the provider.
3. Previews the upgrade, and refuses it, without changing anything, when it
   would replace or delete any of the cluster's resources.
4. Upgrades the cluster:
   - **EKS:** the control plane, then the node group, whose nodes EKS
     replaces one at a time, then the add-ons: `vpc-cni`, `coredns`,
     `kube-proxy` and `aws-ebs-csi-driver`, each to the version EKS gives for
     the new Kubernetes version. The cluster autoscaler is then updated to the
     release for the new version. A control plane EKS has already moved past
     the step's version, as it does when the version leaves standard support,
     keeps its version, as do the add-ons, and only the node group is
     upgraded.
   - **GKE:** the control plane, to its release channel's release of the
     version. GKE upgrades the nodes after it, within the maintenance window.

Progress and failures are on the EKS or GKE runtime instance: the version the
cluster runs in `KubernetesVersion`, and what the controller last did in
`UpgradeStatus`. A refused or failed upgrade is also recorded as a warning
event on the instance. A failed upgrade is tried again after 15 minutes. A
step that failed or was interrupted after the cluster reached its version,
such as one whose add-ons failed to update, is run again on that version to
finish its work before the upgrade is recorded as done. A
refused one isn't tried again until the definition asks for another version,
or `UpgradeRefused` is set back to `false` on the instance. A step run again
to finish an upgrade's work that is refused is recorded as failed as well,
so once `UpgradeRefused` is set back to `false` it runs again.
`UpgradeRequestedVersion` records the version each instance was last asked
for. A cluster Threeport didn't create, such as one `tptctl up
--control-plane-only` was installed on, isn't upgraded.

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
doesn't have to be in the pool. A range larger than a `/16` is reserved whole,
and the cluster's network is laid out in its first `/16`, since AWS and OCI
allow no larger VPC or VCN.

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
| EKS | the VPC is the block's first `/16`; a `/18` private subnet per zone at `.0.0`, `.64.0` and `.128.0`, for nodes and pods; a `/22` public subnet per zone at `.192.0`, `.196.0` and `.200.0`; services in `172.20.0.0/16` |
| GKE | pods in the `/17` at `.0.0`, a `/25` for each of 256 nodes, which run up to 64 pods each; nodes in the `/20` at `.128.0`; services in the `/20` at `.144.0` |
| OKE | the VCN is the block's first `/16`; the API endpoint, node and load balancer subnets where they have always been, relative to it; pods and services in `10.244.0.0/16` and `10.96.0.0/16` |

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
have allocated from it. A child that gives a child of its own a pool must give
it one inside its own pool, since its parent keeps only that pool clear of the
ranges it allocates.

The clusters a child allocated from its pool live in the child's database, and
may outlive the child. So when a child with a pool is deleted, its pool is
recorded in `ReservedNetworkCidrs` on the control plane instance that
represents its parent, and the parent doesn't allocate from it or accept a
runtime range or child pool that overlaps it. Once the clusters allocated from
a reserved block are gone, release it by removing it from
`ReservedNetworkCidrs`: get the control plane instance with `tptctl get
control-plane-instance -o yaml`, remove the block, and apply the file with
`tptctl replace control-plane-instance`. A config file that leaves
`ReservedNetworkCidrs` out keeps the reservations as they are. A child control
plane releases the blocks reserved on its own instance the same way. If
another child was deleted after you got the instance, the replace is refused,
rather than release that child's pool without anyone asking: get the instance
again and repeat.

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

