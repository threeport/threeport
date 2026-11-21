# Control Plane Storage

A control plane keeps its data on two kinds of volume: each CockroachDB node
stores the Threeport API's database on one, and each NATS server stores the
notification streams and the controllers' lock buckets on another. A full
database refuses writes, and a full NATS server refuses every message
published to it, which stops the API from notifying the controllers of
changes. This page covers how large the volumes are, how to grow them, and the
warning Threeport gives before NATS runs out of space.

## Volume Sizes

| | Development | Production |
|---|---|---|
| Each NATS server | 5Gi | 250Gi |
| Each CockroachDB node | 20Gi | 1Ti |

A production control plane runs three NATS servers and three CockroachDB
nodes, so it provisions 750Gi of NATS volumes and 3Ti of database volumes. See
your provider's block storage pricing for what that costs in your region.

`tptctl up` takes other sizes, on either tier:

```bash
tptctl up \
    --provider eks \
    --tier production \
    --message-broker-storage-size 100Gi \
    --database-storage-size 500Gi \
    ...
```

The sizes apply to the volumes an install creates. A control plane that's
reinstalled or upgraded keeps its volumes, and their sizes: grow them as below.

## Storage Classes

Whether a volume can grow in place depends on its StorageClass.

| Provider | Class | Expands in place |
|---|---|---|
| EKS | `threeport-control-plane`, which Threeport creates: gp3 EBS volumes | Yes |
| GKE | the cluster's default, `standard-rwo` | Yes |
| OKE | the cluster's default | `oci-bv` does; the older `oci` FlexVolume class doesn't |
| kind | the cluster's default, `standard` | No |

On EKS, a control plane installed before Threeport created
`threeport-control-plane` has its volumes in `gp2`, the class EKS creates,
which doesn't allow expansion. Allow it before growing the volumes:

```bash
kubectl patch storageclass gp2 -p '{"allowVolumeExpansion": true}'
```

`kubectl get storageclass` shows whether a class allows expansion in its
`ALLOWVOLUMEEXPANSION` column.

## Grow a Volume

Each server's and node's volume is its own PersistentVolumeClaim in the
control plane's namespace: `datadir-nats-js-0`, `datadir-nats-js-1` and so on
for NATS, and `datadir-crdb-0`, `datadir-crdb-1` and so on for CockroachDB.

1. Raise the size each claim requests, one claim at a time:

    ```bash
    kubectl -n threeport-control-plane patch pvc datadir-crdb-0 \
        -p '{"spec":{"resources":{"requests":{"storage":"2Ti"}}}}'
    ```

    The volume grows, and its file system with it, while its pod runs. A
    claim can only grow.

2. Wait until the claim's capacity shows the new size:

    ```bash
    kubectl -n threeport-control-plane get pvc datadir-crdb-0
    ```

3. For NATS, restart each server once its volume has grown, one at a time,
   waiting for each to be ready again:

    ```bash
    kubectl -n threeport-control-plane delete pod nats-js-0
    kubectl -n threeport-control-plane wait --for=condition=Ready pod/nats-js-0
    ```

    A NATS server sets how much it stores from its volume's free space when it
    starts, three quarters of it, so it uses the larger volume only once
    restarted. CockroachDB uses the larger volume without a restart.

The StatefulSets keep the size they were created with in their volume claim
templates, which Kubernetes doesn't allow to change. That size only applies to
a claim the StatefulSet creates, and every server and node has its claim
already.

An EBS volume can be modified once every six hours, so grow it enough the first
time.

## Running Out of Space

The Threeport agent on the control plane's cluster checks each NATS server's
storage every five minutes. When a server uses 80% or more of the space it
stores in, the agent records a `Warning` event on the server's pod, and logs it:

```bash
kubectl -n threeport-control-plane get events \
    --field-selector reason=MessageBrokerStorageNearlyFull
```

```
LAST SEEN   TYPE      REASON                           OBJECT          MESSAGE
2m          Warning   MessageBrokerStorageNearlyFull   pod/nats-js-0   message broker server nats-js-0 uses 82% of its JetStream storage (153.2GiB of 186.5GiB); it refuses messages once it is full, which stops the control plane's notifications: grow its volume before then
```

The event repeats while the server stays above 80%. Grow its volume as above.
Threeport doesn't check the database's volumes: CockroachDB reports each
node's capacity and use in its DB Console and its metrics.
