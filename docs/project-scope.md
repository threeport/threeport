# Threeport Scope

This document lays out the intended scope for the Threeport project.

## In Scope

Threeport is intended to be the engine for an application platform.  It's
purpose is to provide developers with abstractions that allow them to reliably,
repeatably and effectively deliver the software they develop.  The core
Threeport abstractions are intended to cover common use cases and
software design implementations that can be used by a large portion of the
ecosystem to use out-of-the-box.

Additionally, Threeport provides tools for platform engineers to provide custom
abstractions for developers where needed.

The core developer abstractions should cover the following areas:

* Workload deployments.
* Workload dependency management, including:
    * Infrastructure dependencies: cloud provider compute infrastructure.
    * Runtime dependencies: Kubernetes clusters (and alternative runtimes in the
      future).
    * Managed service dependencies (provided by 3rd parties).
    * Support service dependencies (installed on Kubernetes).

### Out of Scope

There are many components of an application platform that are not intended to be
included in the Threeport project.

This applies to any component that is specific to an organization's policies,
procedures and implementations.  It is expected that internal platform
engineering teams use the Threeport SDK to implement these extensions.

This also applies to specific application abstractions.  It is our hope that
extensions for specific apps or implementation patterns be developed in the
open source community.

Examples:

* Integrations with internal project management systems.
* Authentication with 3rd party identity providers.
* Authorization controls for users.
* Automated compliance reporting.
* Specific application abstractions.

