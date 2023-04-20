# Developer Documentation

To try get started with a development environment, see our [Quickstart
Guide](quickstart.md).

## Overview

The threeport repo contains the core components of the threeport control plane.
In addition, this repo contains the threport command line tool `tptctl`, the
developer command line tool `tptdev` and a client library for the threeport API.

Following is an overview of what lives in the root directories of this repo:
* the `bin` directory is where binary artifacts are stored when built
* the `cmd` directory contains the main package for each program that produces a
  binary:
  * [codegen](cmd/codegen/README.md) generates code for various components.
  * [rest-api](cmd/rest-api/README.md) is the RESTful API for the threeport
    control plane.
  * [tptctl](cmd/tptctl/README.md) is the primary client CLI for threeport.
  * [tptdev](cmd/tptdev/README.md) is a developer tool for threeport.
  * [workload-controller](cmd/workload-controller/README.md) is the threeport
    controller that manages containerized workloads on Kubernetes for users.
* the `docs` directory contains these developer docs.
* the `example` directory contains example configurations for testing threeport.
* the `hack` directory contains ad hoc scripts and utilities that have not made
  into a real package or dev tool.
* the `internal` directory contains packages that are used internally by core
  threeport components only.
* the `pkg` directory contains packages that are used by threeport and can be
  imported into other projects.
* the `test` directory contains testing components.

## Core Components

The threeport control plane core components consist of the RESTful API and the
various controllers that provide the logic and functionality for the system.  In
addition there are two 3rd party components:
* [CockroachDB](https://github.com/cockroachdb/cockroach) serves as the
  persistence layer for the threeport API.
* [NATS](https://github.com/nats-io/nats-server) is the message broker used to
  relay notifications from the API to controllers, and by the controllers to
  place distributed locks on objects being reconciled.

## Makefile

This contains a collection of helpful developer make targets.  Run `make` to get
a list of the available operations.

## Packages

Following is an index of package documentation:
* [`internal/api`](../internal/api/README.md)
* [`internal/codegen`](../internal/api/README.md)
* [`internal/kube`](../internal/kube/README.md)
* [`internal/log`](../internal/log/README.md)
* [`internal/provider`](../internal/provider/README.md)
* [`internal/threeport`](../internal/threeport/README.md)
* [`internal/tptdev`](../internal/tptdev/README.md)
* [`internal/version`](../internal/version/README.md)
* [`internal/workload`](../internal/workload.README.md)
* [`pkg/api`](../pkg/api/README.md)
* [`pkg/client`](../pkg/client/README.md)
* [`pkg/config`](../pkg/config/README.md)
* [`pkg/controller`](../pkg/controller/README.md)

