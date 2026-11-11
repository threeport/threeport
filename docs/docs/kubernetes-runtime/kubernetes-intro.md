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
local development.  You can also specify the node sizes and profiles.
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

Reference:
[KubernetesRuntimeInstance](https://pkg.go.dev/github.com/threeport/threeport/pkg/api/v0#KubernetesRuntimeInstance)

## Next Steps

We have a [Remote Kubernetes Runtime](remote-kubernetes-runtime.md) guide that
walks you through the creation of a Kubernetes cluster to use for your workloads
in Threeport.

