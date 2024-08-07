# Control Plane

This document provides a description of the technologies and open source
projects used in Threeport.

This document includes a description each component of the Threeport control
plane, including the technologies and open source projects in use.

## RESTful API

The Threeport API is the heart of the control plane.  All clients and control
plane components coordinate their activity and store persistent data through the
API.

The API is written with the [Go programming
language](https://go.dev/).  We chose Go because of its portability,
efficiency, built-in concurrency, standard library and ecosystem of 3rd party
libraries.  It has become the default programming language for cloud native
systems and has been used extensively in open source projects like Docker and
Kubernetes.

We use the [Echo](https://echo.labstack.com/) framework because it has useful,
performant routing and middleware, is easily extensible and does not contain
excessive, obstructive features.

## API Database

The Threeport API uses [CockroachDB](https://github.com/cockroachdb/cockroach)
for data persistence.  We chose to use a SQL database in general for its
transactional and relational capabilities.  We chose CockroachDB in particular
for its distributed capabilities.  Threeport offers a global control plane so
resilience is a primary concern.  We found CockroachDB to be the best
implementation of a distributed SQL database.

## Notification Broker

The horizontal scalability of Threeport controllers is enabled by the [NATS
messaging system](https://github.com/nats-io/nats-server).  The API uses the
NATS server to notify controllers of changes in the system.  The controllers use
NATS to re-queue reconciliation as needed (when unmet conditions prevent
immediate reconciliation) and to place distributed locks on particular objects
during reconciliation.

## Threeport Agent

The Threeport agent is a [Kubernetes
operator](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/)
built using the [Kubebuilder
SDK](https://github.com/kubernetes-sigs/kubebuilder).  It is informed about
Threeport-managed workloads using a custom Kubernetes resource.  It then places
watches on those resources and collects Events in Kubernetes to report back
status on those workloads to the Threeport API.

You can find more information about the Threeport agent in the [Threeport
developer
documentation](https://github.com/threeport/threeport/blob/main/docs/threeport-agent.md).

## Threeport Controllers

Threeport controllers provide the logic and state reconciliation for the control
plane. They are written in Go and model some engineering principles from
[Kubernetes
controllers](https://kubernetes.io/docs/concepts/architecture/controller/).
When a change is made to an object in the Threeport API, the relevant controller
is notified so that it can reconcile the state of the system with the desired
state configured for that object.

The primary feature that differentiates
Threeport controllers from those in Kubernetes is that Threeport controllers are
horizontally scalable.  Threeport does not not use a
[watch](https://kubernetes.io/docs/reference/using-api/api-concepts/#efficient-detection-of-changes)
mechanism the way Kubernetes does.  Instead we use a notification broker
that allows notifications to be provided to only one of a set of identical
controllers at a time.

### AWS Controller

The AWS controller is responsible for managing the following managed services in
AWS:

* Elastic Kubernetes Clusters (EKS): used for Kubernetes Runtime environments to
  deploy user workloads.
* Relational Database Service (RDS): available as a dependency when used as a
  part of an application stack.
* Simple Storage Service (S3): available as a dependency when used by an
  application to store objects.

We use a library called [aws-builder](https://github.com/nukleros/aws-builder)
that was developed for use by Threeport.  It uses the [v2
SDK for the Go programming language](https://github.com/aws/aws-sdk-go-v2) to
manage AWS resources.  We do not use any intermediate toolchains or libraries
such as [Pulumi](https://github.com/pulumi/pulumi), [ACK](https://github.com/aws-controllers-k8s/community),
[Crossplane](https://github.com/crossplane/crossplane) or [Terraform](https://github.com/crossplane/crossplane).
These are capable tools for certain
use cases.  However, using the AWS SDK directly gives us the most flexibility
and ensures we don't encounter any unsupported operations we might need to
perform in managing cloud resources for Threeport users.  It also serves as a
reference implementation for platform engineers that extend Threeport and wish
to use a similar approach.

### Control Plane Controller

The control plane controller allows users of Threeport to deploy new Threeport
control planes using Threeport itself.  This is available so that large organization
that wish to clearly separate concerns between lines of business can do so in a
Threeport-native way without the need to use `tptctl` to bootstrap new control
planes when needed.  It also allows separation of tiers, e.g. development and
production if desired.  However, we don't recommend this approach for most users
unless they have a compelling need for it.

### Gateway Controller

The gateway controller manages network ingress support services when a workload
has such a dependency.

The following support services are installed on Kubernetes as needed by the
Gateway controller:

* [Gloo Edge](https://github.com/solo-io/gloo): the network ingress
controller used by Threeport.
* [cert-manager](https://github.com/cert-manager/cert-manager): used to
provision and rotate TLS certificates.
* [external-dns](https://github.com/kubernetes-sigs/external-dns): manages DNS
records created for workloads.

When a support service controller needs to be installed in Kubernetes, we use
the
[support-services-operator](https://github.com/nukleros/support-services-operator)
to perform the install.  The Kubernetes manifest provided to the Workload is
actually a custom resource that is managed by the support-services-operator.  It
installs the support services listed above in this manner.

In addition to the support services installations on Kubernetes, the gateway
controller appends Kubernetes resources to those defined by the user with the
Workload resource to configure the support service for that workload.

### Helm Workload Controller

The Helm workload controller uses the popular Kubernetes package manager,
[Helm](https://helm.sh/) to deploy workloads in Kubernetes.  Helm templates have
drawbacks in complex environments since templating is inherently inferior to
general purpose programming languages.  More on this topic is discussed in the
[Continuous Delivery & GitOps
section](../concepts/application-orchestration.md#continuous-delivery-gitops)
of the Application Orchestration document.  We prefer Go programs to construct
Kubernetes resources.  However, Helm support is still valuable because many open-source
charts are available and it is already in extensive use by many teams.

Although we prefer Go to manage Kubernetes resources, we recognize there are
use-cases where it is more appropriate to use Helm. Threeport's implementation
of the [Observability Stack](../../observability/observability-intro/) is one
example.  The requirements for observability line up well with what is already
provided by open-source Helm charts, so it made more sense to implement this
controller with Threeport's Helm integration.

### Kubernetes Runtime Controller

The Kubernetes runtime controller is used to provision new Kubernetes
environments for workloads.  It serves as a cloud provider agnostic abstraction
that allows a user to provision environments with the cloud provider as a simple
attribute of the `KubernetesRuntimeDefinition` object.  The
`KubernetesRuntimeInstance` object is where connection information for each
cluster is stored and utilized when workloads are deployed to that cluster.

### Observability Controller

The observability controller is responsible for deploying observability stacks
when a Threeport user wishes to have access to metrics and logs from their
workloads.

The following components are available to deploy as a part of the stack:

* [Prometheus](https://prometheus.io/docs/introduction/overview/): the metrics
  collection system.
* [Promtail](https://grafana.com/docs/loki/latest/send-data/promtail/): log
  forwarding from individual machines to the log storage back end.
* [Loki](https://github.com/grafana/loki): log storage.
* [Grafana](https://github.com/grafana/grafana): observability dashboard.

The observability controller leverages the Helm workload controller to install
Helm charts to deploy each of these components.

### Secrets Controller

The secrets controller is responsible for storing sensitive information in a
secret storage system.  Currently, the only supported managed service for this
is AWS Secret Manager.  The secret manager leverages the
[external-secrets](https://github.com/external-secrets/external-secrets)
project.  This support service is also installed by the
support-services-operator when needed.  This allows Threeport to expose secrets
to running apps as needed by users.

> Note: when storing secrets using Threeport, the value of the secret is never
> stored in the Threeport database.  The notifications that contain the secret
> value are never written to disk by NATS and are encrypted in transit.

### Terraform Controller

The Terraform controller uses [Terraform](https://www.terraform.io/) to
provision custom infrastructure needed by workloads.  Terraform is less than
ideal for provisioning infrastructure in a control plane like Threeport for
reasons discussed [elsewhere](../concepts/application-orchestration.md#continuous-delivery-gitops)
but it is offered in Threeport for two reasons.  Many teams have made extensive
use of Terraform and this allows them to use those configs in Threeport.  Also,
Threeport offers native support for only a small number of AWS managed services
and Terraform offers support for a much larger number of those
resources.  Using Terraform for simpler use cases can be useful compared to the
alternative of developing a custom Threeport extension to manage those
same AWS resources.

> Note: Terraform is only supported for managing AWS resource at this time.

### Workload Controller

The workload controller deploys a defined set of Kubernetes resources to a
nominated (or default) Kubernetes runtime instance.  This controller is quite
rudimentary in that the user is required to define the granular detail of all
Kubernetes resources that constitute their workload.  However, it is useful in
simple implementations.  When paired with a [Kubernetes operator](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/)
or a custom Threeport controller that abstracts the details of the Kubernetes
resources, it is a vital mechanism.  It is the primary interface with the
Kubernetes API in Threeport.

## Next Steps

For a more depth of understanding in how Threeport controllers work, see our
[Threeport Controllers architecture documentation](threeport-controllers.md).
