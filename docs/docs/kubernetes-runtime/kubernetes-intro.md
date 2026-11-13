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

