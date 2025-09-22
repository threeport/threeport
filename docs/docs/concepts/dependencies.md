# Dependencies

Applications don't exist in a vacuum.
They have dependencies.  Threeport manages all the dependencies for an
application so that a development team can start with absolutely no
infrastructure or runtime environment and readily deploy their application using
Threeport.

Threeport treats the following concerns as application dependencies:

* Cloud Infrastructure: Threeport orchestrates the provisioning, availability
  and autoscaling of all compute and networking infra needed to run an
  application.
* Container Orchestration: Kubernetes is the most capable runtime environment
  for cloud native software.  Threeport installs and configures Kubernetes
  environments for the application being deployed.
* Support Services: Applications need support services installed on Kubernetes
  to provide things like network connectivity, TLS termination, DNS record
  management, secrets, metrics and log aggregation.  Threeport installs and
  configures these support services for each application deployed.
* Managed Services: Many applications use cloud provider and/or commercial
  managed services such as databases or observability services as a part of
  their stack.  Threeport orchestrates the provisioning and connectivity for
  the application's workloads to make these managed services available.

The ultimate end-user of Threeport is the developer or operator that needs to
deploy and manage instances of their applications - for development, testing,
staging or production. The user provides a config for their app that declares
its dependencies.  Threeport orchestrates the delivery of the application
workloads along with all their dependencies.

![Threeport Developer Experience](../img/ThreeportDevExperience.png)

## Next Steps

To get a practical understanding of how Threeport manages delivery, check out
our [Getting Started page](../getting-started.md) which provides the steps to
install Threeport and use it to deploy a sample application.

