# tptdev

Manage development operations with ease.

Here you will find the main package for `tptdev` which is a developer tool to help make
developers' lives more efficient.  Currently it supports spinning up and down development
environments, building docker images, and managing debug mode of Threeport control plane
components.

If you find yourself writing scripts or complex make targets for common development tasks,
it may warrant a new command for `tptdev`.

Below is a brief overview of commands offered by tptdev. Use `tptdev $command --help` for
more information about each of them.

## tptdev up

Spins up a developer genesis control plane.

## tptdev down

Spins down a developer genesis control plane.

## tptdev build

Builds docker images that are used by Threeport control plane components.

## tptdev debug

Enable/disable debug mode for Threeport control plane components.

## tptdev reinstall

Sweep and reapply the stateless side of a running dev control plane
(controller and API server pods, configmaps, RBAC).  CockroachDB data,
NATS data, certificates and the rest-api service IP stay in place.

Pass `--drop-database` to empty the schema and NATS streams as well.  That
requires `--confirm` with the control plane name, and the cluster must be
recorded as a development installation.
