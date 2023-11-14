# NATS Notification Management

This document is intended as an aid to engineers developing and operating
Threeport.

Refer to the [NATS docs](https://docs.nats.io/using-nats/nats-tools/nats_cli)
for install instructions and full documentation for the NATS CLI.

## Configuration

### Local Threeport

Create a context for connecting.  This example is to connect to a local NATS
instance when running Threeport on kind using the context name `local`.

```bash
nats context create local
```

For a local Threeport control plane, port forward the NATS connection.

```bash
kubectl port-forward svc/nats-js 4222:4222 -n threeport-control-plane
```

To edit the context for connecting to NATS, the following command will open the
NATS context config file.  (Not necessary for a local instance using the port
forward shown above.)

```bash
nats context edit local
```

### Remote Threeport

With a Threeport control plane running on AWS EKS, the process is similar.

Use the aws CLI to update your local kubeconfig.

```bash
aws eks update-kubeconfig --name [eks cluster name]
```

Now you can run the port forward for NATS JetStream as shown above and use the
NATS CLI.

## Managing Notifications

The NATS Jetstream streams are used to notify controllers of reconciliation
work.

View the available streams.  There will be one for each controller as a rule.

```bash
nats stream ls
╭─────────────────────────────────────────────────────────────────────────────────────────────────╮
│                                             Streams                                             │
├─────────────────────────┬─────────────┬─────────────────────┬──────────┬─────────┬──────────────┤
│ Name                    │ Description │ Created             │ Messages │ Size    │ Last Message │
├─────────────────────────┼─────────────┼─────────────────────┼──────────┼─────────┼──────────────┤
│ controlPlaneStream      │             │ 2023-11-09 07:11:54 │ 0        │ 0 B     │ never        │
│ gatewayStream           │             │ 2023-11-09 07:11:54 │ 0        │ 0 B     │ never        │
│ workloadStream          │             │ 2023-11-09 07:11:54 │ 0        │ 0 B     │ never        │
│ kubernetesRuntimeStream │             │ 2023-11-09 07:11:54 │ 6        │ 8.9 KiB │ 35m21s       │
│ awsStream               │             │ 2023-11-09 07:11:54 │ 36       │ 73 KiB  │ 37m21s       │
╰─────────────────────────┴─────────────┴─────────────────────┴──────────┴─────────┴──────────────╯
```

Each stream will use subjects for notifications - one subject for each operation
on a distinct object.  The following command displays the subjects and the
number of messages for each subject on a particular stream.

```bash
nats stream subjects awsStream
  awsEksKubernetesRuntimeInstance.delete: 1
  awsEksKubernetesRuntimeInstance.create: 2
  awsEksKubernetesRuntimeInstance.update: 33
```

In this example there was one message for deleting an EKS runtime instance, 2
for creating and 33 update messages.

The following command displays the create notifications for EKS runtime
instances.

```bash
nats stream view awsStream --subject awsEksKubernetesRuntimeInstance.create
[1] Subject: awsEksKubernetesRuntimeInstance.create Received: 2023-11-09T07:18:36-05:00

{"Operation":"Created","CreationTime":1699532316,"Object":{"ID":915740806419742721,"CreatedAt":"2023-11-09T12:18:36.020403Z","UpdatedAt":"2023-11-09T12:18:36.020403Z","Name":"eks-remote-0","Reconciled":false,"CreationFailed":false,"InterruptReconciliation":false,"Region":"us-east-1","AwsEksKubernetesRuntimeDefinitionID":915740803044081665,"KubernetesRuntimeInstanceID":915740802923855873}}

[18] Subject: awsEksKubernetesRuntimeInstance.create Received: 2023-11-14T10:14:32-05:00

{"Operation":"Created","CreationTime":1699974872,"Object":{"ID":917190976565575681,"CreatedAt":"2023-11-14T15:14:32.828335Z","UpdatedAt":"2023-11-14T15:14:32.828335Z","Name":"eks-remote-1","Reconciled":false,"CreationFailed":false,"InterruptReconciliation":false,"Region":"us-east-1","AwsEksKubernetesRuntimeDefinitionID":915740803044081665,"KubernetesRuntimeInstanceID":917190976499875841}}

11:13:18 Reached apparent end of data
```

You can also get a single notification by message ID.

```bash
nats stream get awsStream 18
Item: awsStream#18 received 2023-11-14 15:14:32.829734966 +0000 UTC on Subject awsEksKubernetesRuntimeInstance.create

{"Operation":"Created","CreationTime":1699974872,"Object":{"ID":917190976565575681,"CreatedAt":"2023-11-14T15:14:32.828335Z","UpdatedAt":"2023-11-14T15:14:32.828335Z","Name":"eks-remote-1","Reconciled":false,"CreationFailed":false,"InterruptReconciliation":false,"Region":"us-east-1","AwsEksKubernetesRuntimeDefinitionID":915740803044081665,"KubernetesRuntimeInstanceID":917190976499875841}}
```

_Note_: When the Threeport API notifies a controller with a message, if the controller
requeues the message for future re-reconciliation, that message is negatively
acknowledged with a delay, telling NATs to redeliver the message after the
prescribed delay.  This applies to the same message so the message ID doesn't
change when it's requeued.

The NATS CLI does not seem to have a command to acknowledge a particular message
by ID.  The only way to interrupt a controller from receiving a message that has
been requeued is to remove it.  Be certain this is what you want to do.

Remove a message that you've identified by message ID.  This will prompt you to
confirm that you really want to remove the message.  When you confirm the
message will be permanently removed.

```bash
nats stream rmm awsStream 37
```

## Managing Locks

The NATS JetStream key-value store is used for locks on reconciling objects.

To view the lock buckets in use.  The Values column shown in the output below
indicates locks have been used on aws and kubernetes runtime objects (as you
would expect when spinning up an EKS runtime).  The part that is a little tricky
is that it doesn't necessarily mean the lock is active when Values are shown.

```bash
nats kv ls
╭───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮
│                                                     Key-Value Buckets                                                     │
├───────────────────────┬──────────────────────────────────────────────┬─────────────────────┬───────┬────────┬─────────────┤
│ Bucket                │ Description                                  │ Created             │ Size  │ Values │ Last Update │
├───────────────────────┼──────────────────────────────────────────────┼─────────────────────┼───────┼────────┼─────────────┤
│ controlPlaneLock      │ contains locks on control plane objects      │ 2023-11-09 12:12:24 │ 0 B   │ 0      │ never       │
│ gatewayLock           │ contains locks on gateway objects            │ 2023-11-09 12:12:19 │ 0 B   │ 0      │ never       │
│ workloadLock          │ contains locks on workload objects           │ 2023-11-09 12:12:06 │ 0 B   │ 0      │ never       │
│ awsLock               │ contains locks on aws objects                │ 2023-11-09 12:12:16 │ 137 B │ 1      │ 34.34s      │
│ kubernetesRuntimeLock │ contains locks on kubernetes runtime objects │ 2023-11-09 12:12:10 │ 145 B │ 1      │ 34.82s      │
╰───────────────────────┴──────────────────────────────────────────────┴─────────────────────┴───────┴────────┴─────────────╯
```

In order to see any locks that are currently in place, list the keys in a
bucket.  If the response is `No keys found in bucket` there are no active locks.
In order to find if a lock is active for a particular object, refer to the
objects unique ID in the lock key (see examples below).

```bash
nats kv ls awsLock
```

In order to see any locks being placed and removed, as when reconciliation
occurs, watch the bucket.  If reconciliation occurs you will see output similar
to below.

```bash
nats kv watch awsLock
[2023-11-14 10:28:33] PUT awsLock > AwsEksKubernetesRuntimeInstanceReconciler.917190976565575681: c75aacb0-328b-4ac5-b466-cf731016f605
[2023-11-14 10:28:33] DELETE awsLock > AwsEksKubernetesRuntimeInstanceReconciler.917190976565575681
```

The PUT operation locked the AwsEksKubernetesRuntimeInstance with database
unique ID `917190976565575681` as shown in the key.  The value is the UUID for
the specific controller instance that placed and then removed the lock.

The keys and values can be correlated to the controller logs which contain a
`controllerID` (the UUID) and the Threeport object ID.

If you need to release a lock because, say, a controller placed a lock, crashed
and never released it, use the following command to remove the record by
deleting the key.

```bash
nats kv rm AwsEksKubernetesRuntimeInstanceReconciler.917190976565575681
```

