# Comparisons

Following are some comparisons between Threeport and other projects and
products.

## Kubernetes Distributions

There are many vendors that provide installation of Kubernetes with various
support services offered as optional installs on Kubernetes.  Some of them also
offer GitOps systems that can be used to deploy workloads.

The workflow when using a Kubernetes distribution is generally as follows:

1. A platform team use the Kubernetes distro to install clusters and prepare
   them for use.
1. DevOps sets up CI/CD or GitOps pipelines to deploy into those clusters.
1. Developers push changes to config repos that trigger delivery of workloads to
   the clusters.  If any mishaps occur in the pipeline, DevOps is usually
   consulted to troubleshoot.

Threeport provides Kubernetes clusters as well, but it installs support services
as they are needed by workloads.  And the clusters need not be provisioned by
the platform team ahead of time.  They can be spun up on-demand by teams that
need them.

The core Threeport project does not support any particular vendor distro, but
can be extended using the [Threeport SDK](../sdk/sdk-intro.md) to support any
distro as the provider of Kubernetes clusters.  With Threeport, the workflow is
different:

1. The platform team installs the Threeport control plane.
1. The DevOps team provides definitions and defaults for Kubernetes runtimes
   and/or dependencies that can be used by dev teams.
1. Developers provide workload configs with dependency declarations to
   Threeport.  Threeport orchestrates the deployment of the application and its
   dependencies.

Threeport enables a true self-service experience for dev teams that provides
efficiency and velocity not available when humans are on the critical delivery
path for applications.

## CI/CD

Continuous integration and continuous delivery are very different concerns.  The
only reason these two operations are mashed into a single term is that pipeline
systems like Jenkins were used to implement them.

### Continuous Integration

CI is concerned with automating the tests, security scans and builds for pieces
of software.  Generally, when a pull request is opened to make changes to a
codebase, these operations are run so that when code is merged a reliable
artifact can be made available for delivery.  In cloud native systems, the final
artifact is a container image that can be pulled to a runtime environment and
deployed.

There are many CI solutions, such as GitHub Actions that are commonly configured
and managed by dev teams as a software development concern.

Threeport is not involved in the continuous integration processes.

### Continuous Delivery

CD uses similar version control triggers and pipeline-based mechanisms but has
the distinct job of managing complex configurations for software to run in
compute environments.  There are far more dynamic and complex concerns at play
in delivery than there are in the well-understood domain of tests, scans and
builds for a particular software project.

GitOps is a more recent evolution of this concept that wrangles complex
infrastructure and Kubernetes configuration for workloads.  Those configurations
are managed by humans and stored in version control and leverage CLI tools to
perform templating, overlays, and API calls to cloud provider and Kubernetes
APIs to deliver apps to their runtime.  Due to their size and complexity, these configs
usually live in their own distinct git repositories, separate from the app's
source code.  This additional complexity and human involvement adds considerable
overhead to dev teams and/or requires dedicated DevOps teams to manage.

Threeport provides an alternative to continuous delivery systems.  It stores
user-defined config in a database, rather than a git repo.  And it uses a
purpose-built control plane to deliver software rather than a pipeline with a
set of CLI tools that were designed for humans to use and are not natively
interoperable.

## Radius

[Radius](https://radapp.io/) helps teams manage cloud native application
dependencies.

Similarities:

* Both Threeport and Radius have a strong emphasis on providing developer
  abstractions that allow workloads to be deployed _with_ their dependencies,
  such as managed services like AWS RDS and S3.  Radius' support for a wide
  range of managed services is more mature than in Threeport.
* Both Threeport and Radius are fundamentally multi-cloud systems.  Threeport
  only supports AWS today - but it is designed to have other cloud provider
  support plugged in.  Radius offers support for AWS and Azure today.
* Both Threeport and Radius aim to provide a platform for collaboration between
  developers and other IT operators.  Developers need ways to smoothly leverage
  the expertise offered by other teams with minimal friction.

Differences:

* Radius does not manage Kubernetes clusters.  To get started with Radius, you
  must have a Kubernetes cluster.  In contrast, Threeport manages Kubernetes
  clusters as runtime dependencies.
* Threeport manages support services that must be installed on Kubernetes as
  application dependencies.  Examples include network ingress routing, TLS
  termination and DNS management.  These common support services are installed
  and configured for tenant applications by Threeport.  With Radius, support
  services can be installed, but configuration of them is up to the user to
  manage.
* Radius has a strong emphaisis on leveraging existing tools like Bicep, Helm
  and Terraform and unifying them in a common platform.  Threeport supports Helm
  and Terraform, but encourages migrating towards the use of programming
  languages like Go to manage resource configuration.  The Threeport SDK (coming
  soon) allows a smooth transition from DevOps tools to controllers for
  accomplishing this.

Radius and Threeport have very complimentary characteristics and could be
combined well.

## Crossplane

[Crossplane](https://www.crossplane.io/) provides a framework for building
customizations to the Kubernetes control plane.

Similarities:

* Both Threeport and Crossplane facilitate building custom application
  platforms.
* Threeport manages workload dependencies, such as managed services, as a
  primary function.  Similar functionality can be built out with Crossplane.

Differences:

* Crossplane aims to build custom Kubernetes control planes without needing to
  write code.  This is achieved with compositions that define new APIs in YAML.
  In contrast, platform engineers extend Threeport by writing code.  We believe
  that languages like Go are a better choice for implementing sophisticated
  software systems.  As such, we offer the [Threeport SDK](../sdk/sdk-intro.md)
  that allows users to build their custom implementations with Go, rather than
  with compositions defined in YAML.
* Crossplane is an extension of the Kubernetes control plane.  The Threeport control
  plane is a distinct control plane with its own APIs.  The Threeport control
  plane supports greater scalability and geo-redundancy than Kubernetes so as to
  serve as a global control plane for all clusters under management.

Crossplane and Threeport could be used in conjunction by using Threeport to
provision and manage Kubernetes with Crossplane extensions.  However, there are a
lot of overlapping concerns between the projects.  Building an application platform
using both projects would introduce more complexity and unclear boundaries.

## ArgoCD

[Argo CD](https://argoproj.github.io/cd/) is a modern Kubernetes-native
continuous delivery system.

Similarities:

* Both ArgoCD and Threeport manage software delivery.

Differences:

* ArgoCD supports various DevOps tools to be used in workflows to execute the
  steps needed to deliver software.  Threeport instead uses software
  controllers to manage software delivery.  With ArgoCD you can get a delivery
  pipeline up and running pretty quickly.  The challenge is maintainability when
  complexity increases.  When using Helm charts with Kustomize overlays for
  sophisticated distributed applications, the complexity overhead can become
  quite a burden.  Threeport advocates using code in a software controller
  instead of config languages in a pipeline.  This means more work up-front and
  changes to the delivery system are a bit more involved.  However, this
  approach improves the maintainability of complex delivery systems.
* ArgoCD generally pulls configuration from Git repos and applies them to
  Kubernetes clusters.  Threeport uses a relational database to store config
  which provides more efficient access to software controllers that need to both
  read and write configuration details.

ArgoCD and Threeport could be used in conjunction by using Threeport to
provision and manage Kubernetes clusters with ArgoCD.  However, similar to
Crossplane, there are a lot of overlapping concerns between the projects.  Using
Crossplane and ArgoCD together make far more sense than using Threeport with
either Crossplane or ArgoCD.

## Next Steps

If you'd like to try out Threeport for yourself, visit our [getting started
guide](../getting-started.md).

If you'd like to learn about the architecture, check out our [architecture
overview](../architecture/overview.md).

