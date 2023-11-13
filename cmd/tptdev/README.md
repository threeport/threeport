# tptdev

Manage development operations with ease.

Here you will find the main package for `tptdev` which is a developer tool to help make
developers' lives easier.  Currently it supports spinning up and down development
environments, building docker images, and managing debug mode of threeport control plane
components.  The dev environment differs from a regular instance of threeport in that the
API and controller components are created with your local code repo mounted. Code changes
can optionally be live-reloaded into a control plane component by enabling it via `tptdev
debug`.

If you find yourself writing scripts or complex make targets for common development tasks,
it may warrant a new command for `tptdev`.

Below is a brief overview of commands offered by tptdev. Use `tptdev $command --help` for
more information about each of them.

## tptdev up

Spins up a developer genesis control plane.

## tptdev down

Spins down a developer genesis control plane.

## tptdev build

Builds docker images that are used by Threeport control plane components. In order to push
docker images, you must set `DOCKER_USERNAME` and `DOCKER_PASSWORD` environment variables.

## tptdev debug

Enable/disable debug mode for Threeport control plane components.
