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

Note: run every command in this guide, including all mage targets, from the
root of this repo.

First, check out the branch you'll be working from - generally the latest
release branch (currently `0.7`), or a feature branch checked out from it if
you're working on a contribution.  Then install the `tptctl` CLI by building
it from that branch's source with mage.  This ensures `tptctl` matches the
code you're working on:

```bash
mage install:tptctl
```

This prints the exact path it installed to, e.g. `tptctl binary installed
and available at /home/you/go/bin/tptctl`.  If `tptctl version` fails with
"command not found" afterward, that directory isn't on your `PATH` - add it,
using the directory from your own command's output (not copied verbatim):

```bash
export PATH="$PATH:/home/you/go/bin"
```

Add that line to your shell profile (e.g. `~/.zshrc` or `~/.bashrc`) to
persist it across sessions.

Create a local container registry.  This mage target runs a local docker
container to serve as the container registry, so we don't have to wait for
images to be pushed to - and pulled from - a remote registry.

```bash
mage dev:localRegistryUp
```

Build all Threeport control plane images and push them to the local registry.
With no overrides, `mage build:allImages` pushes to `localhost:5001` and tags
each image with the current version plus the short commit sha.

```bash
mage build:allImages
```

Install Threeport from the newly built images.  `--name` is an arbitrary name
you choose for this control plane instance.  Omit `--control-plane-image-tag`
so `tptctl up` resolves the same sha-suffixed tag the build pushed.
`--control-plane-image-namespace` is always `localhost:5001` for this
local-registry workflow:

```bash
CONTROL_PLANE_NAME=dev-0  # an arbitrary name for the Threeport installation

tptctl up \
  --name "$CONTROL_PLANE_NAME" \
  --provider kind \
  --local-registry \
  --control-plane-image-namespace localhost:5001
```

This will start a local kind cluster and install the control plane.  You can now
make calls to the API server.

Call the API.  Note this is a different port than the local container registry
(`localhost:5001`) - the API is served on the default http port:

```bash
curl localhost/swagger/index.html
```

Uninstall the local dev control plane:

```bash
tptctl down --name "$CONTROL_PLANE_NAME"
```

Stop and remove the local container registry:

```bash
mage dev:localRegistryDown
```

