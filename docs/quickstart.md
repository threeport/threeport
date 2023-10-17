## Quickstart

In order to run a local development control plane of threeport, you'll need the
following installed:

* [docker](https://docs.docker.com/get-docker/)
* [kind](https://kind.sigs.k8s.io/)
* [kubectl](https://kubernetes.io/docs/reference/kubectl/)

The following will also need to be installed locally for the relevant development
operations and make targets:

* [swag CLI](https://github.com/swaggo/swag) `>=v1.8.12,<v2.0.0` for generating API docs.
* [NATS CLI](https://github.com/nats-io/natscli) for interacting with NATS
  messages used by the control plane.
* [delve](https://github.com/go-delve/delve) for running debug sessions.

Spin up a local dev control plane:

```bash
make dev-up
```

This will start a local kind cluster and install the control plane.  You can now
make calls to the API server and make local code changes that will be
hot-reloaded in place.

Note: The development environment is created using tptdev tool.  The tptdev
tool references files in the source code so assumes, by default that it is being
run from the root of this repo.

Note: When running dev control planes, the entrypoint process is
[air](https://github.com/cosmtrek/air) which
manages the live reload of code changes on your filesystem.  Therefore, if an
error occurs, the container will not fail and restart.  For example, if the build
fails due to a compile error for a live reload the container status will remain
`Running` because air is still running.  View the pod logs with `kubectl logs`
to see if this is the case. The workload controller, for example, comes up
before the API.  In a dev environment, it will generally need to be restarted
after the API is up to work correctly.

Call the API:

```bash
curl localhost/swagger/index.html
```

Delete a local dev control plane:

```bash
make dev-down
```

