# Kubernetes Federation

This document describes the Threeport approach to managing a fleet of Kubernetes
clusters.

There have been many attempts at federating Kubernetes using Kubernetes itself,
i.e. Kubernetes operators that install and keep an inventory of clusters as well
as manage multi-cluster app deployments.  Kubernetes was designed to be a
data center-level abstraction and it performs this function very well.  It was
not designed to be a global software fleet abstraction and it has inherent
scaling and availability constraints that prevent it from providing the ideal
solution to this concern.

A global federation layer must be highly scalable and have geo-redundant
availability.  A control plane for all your software deployments must have the
appropriate capacity and resilience for the task.

## Kubernetes Controllers

[Kubernetes controllers](https://kubernetes.io/docs/concepts/architecture/controller/)
are not horizontally scalable.  When deployed in a highly
available configuration, only one controller is active at any given time and
they use leader election to determine which of a set of identical controllers
manage operations at any given time.  In many use-cases, many thousands of
clusters must be managed coherently, not to mention the software in those
clusters.  This is a tremendous amount of state reconciliation to be performed by
a single controller that does not share load across multiple instances.

## Kubernetes Data Store

Kubernetes uses [etcd](https://etcd.io/) which is an excellent distributed
key-value store.  It has served Kubernetes very well in its purpose.  However,
etcd works best in a single region.  Tuning for the increased latency of
cross-region etcd clusters is possible, but treacherous.  Furthermore, it is not
a relational database which means if you need transactional capabilities that
allow a database to make changes to multiple objects with ACID guarantees, etcd
is not the best choice.

![Federating Kubernetes with Kubernetes](../img/KubernetesFederationWithKubernetes.png)

## Threeport Controllers

Threeport controllers inherit a lot of design principles from Kubernetes.  They
are level-triggered state reconciliation programs that operate on a non-terminating
loop until the desired state is realized in the system.  One thing that
Threeport controllers add is horizontal scalability.  Any number of Threeport
Controllers can operate simultaneously to manage the same set of object types.
They use NATS Jetstream to broker notifications to help achieve this.  In
Threeport, the message broker helps ensure a notification of a particular change
is delivered to just one of a set of identical Threeport controllers.  Threeport
controllers also use the message broker to place distributed locks on specific
objects while they are being reconciled so that race conditions don't occur
between different replicas of a controller when rapid changes are made to a
particular object.

## Threeport Data Store

Threeport uses CockroachDB, a purpose-built geo-redundant relational database.  The
geo-redundancy is essential for a purpose as critical as a global control plane.  And the
transactional capabilities allow changes to multiple related objects to happen
safely.  When you are dealing with remote clusters and the workloads therein,
changes that affect multiple objects are common.  Being able to apply a change
to all the affected objects _or_ none at all if a problem occurs, is an
important guarantee to have for stability.

![Federating Kubernetes with Threeport](../img/ThreeportKubernetesFederation.png)

