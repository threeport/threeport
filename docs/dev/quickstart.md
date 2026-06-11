## Quickstart

In order to run a local development control plane of threeport, you'll need the
following installed:

* [docker](https://docs.docker.com/get-docker/)
* [kind](https://kind.sigs.k8s.io/)
* [kubectl](https://kubernetes.io/docs/reference/kubectl/)

The following will also need to be installed locally for the relevant development
operations:

* [swag CLI](https://github.com/swaggo/swag) `>=v1.16.2,<v2.0.0` for generating API docs.
* [NATS CLI](https://github.com/nats-io/natscli) for interacting with NATS
  messages used by the control plane.
* [mage](https://magefile.org/) for using mage targets.

Spin up a local dev control plane:

```bash
make dev-up
```

This will start a local kind cluster and install the control plane.  You can now
make calls to the API server.

Note: The development environment is created using tptdev tool.  The tptdev
tool references files in the source code so assumes, by default that it is being
run from the root of this repo.

Call the API:

```bash
curl localhost/swagger/index.html
```

Delete a local dev control plane:

```bash
make dev-down
```

