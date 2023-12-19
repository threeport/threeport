# Threeport Scope

This document lays out the intended scope for the Threeport project.  This scope
is to help limit the functionality included to those that are broadly used -
and likely to be into the future.

## In Scope

Threeport is intended to be the engine for an application platform.  It's
purpose is to provide developers with abstractions that allow them to reliably,
repeatbably and effectively deliver the software they develop.  The core
Threeport abstractions are intended to cover common development patterns and
software design implementations that can be used by a large portion of the
ecosystem to use out-of-the-box.

Additionally, Threeport provides tools for platform engineers to provide custom
abstractions for developers where needed.

The core developer abstractions should cover the following areas:

* Workload deployments
* Workload dependency management, including:
    * Infrastructure depednencies: cloud provider compute infrastructure
    * Runtime dependencies: Kubernetes clusters (and alternative runtimes in the
      future)
    * managed service dependencies (provided by 3rd parties)
    * support service dependencies (installed on Kubernetes)

### Out of Scope

There are many components of an application platform that are not intended to be
included in the Threeport project.  These components are those that are specific
to a particular organization's policies and procedures.

Extensions to support these use cases can be implemented internally by platform
engineering teams and in the open source community to be used by those that need
the integrations or features.

Examples:

* Integrations with internal project management systems
* User access management controls
* Automated compliance reporting

