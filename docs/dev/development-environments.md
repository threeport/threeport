# Development Environments

## Local Control Plane

This will create a threeport control plane locally using `tptdev`.  It will stand
up a kind cluster and install the threeport control plane with mounts to your
local filesystem.  Code changes you make locally will be live-reloaded.

This is good for:
* immediately testing changes
* running e2e tests

To stand up a local control plane:

```bash
make dev-up
```

The drawback here is that if you are running tests you cannot make any code
updates without invoking rebuilds of all components.  For example, a long-running
infrastructure change in a cloud provider will be interrupted.

## Local Static Control Plane

The following steps describe how to stand up a local control plane that will
allow you to make other code changes without invoking live-reloads.

This is good for:
* making changes while running a long-running test
* pre-release testing

In order to stand up a local threeport control plane using the code you have
locally without live-reloads, follow these steps:

1. Set environment variables to declare image registry and tags for the control
   plane images.  The [direnv](https://direnv.net/) tool is useful here.  For
   example, I use my dockerhub account `lander2k2` and the tag `test`.  My
   `.envrc` file looks as follows:

   ```
   export REST_API_IMG=lander2k2/threeport-rest-api:test
   export WORKLOAD_CONTROLLER_IMG=lander2k2/threeport-workload-controller:test
   export KUBERNETES_RUNTIME_CONTROLLER_IMG=lander2k2/threeport-kubernetes-runtime-controller:test
   export AWS_CONTROLLER_IMG=lander2k2/threeport-aws-controller:test
   export GATEWAY_CONTROLLER_IMG=lander2k2/threeport-gateway-controller:test
   export AGENT_IMG=lander2k2/threeport-agent:test
   ```

   Note: the image name, e.g. `threeport-rest-api` must match the image names
   declared as constants in `internal/threeport/components.go`.

1. Build all control plane images in parallel:

   ```bash
   make -j control-plane-images
   ```

1. Use `tptctl` to start a local control plane using your registry and tag.  For
   example:

   ```bash
   make build-tptctl
   tptctl up -n dev-0 -p kind -i lander2k2 -t test
   ```

