# Threeport

An application orchestration control plane.

## Quickstart

In order to run a local development instance of threeport, you'll need:

* [docker](https://docs.docker.com/get-docker/)
* [kind](https://kind.sigs.k8s.io/)
* [kubectl](https://kubernetes.io/docs/reference/kubectl/)

The following may also be required for different development operations:
* [swag CLI](https://github.com/swaggo/swag) for generating API docs.

Spin up a local dev instance:

```bash
make dev-up
```

This will start a local kind cluster and install the control plane.  You can now
make calls to the API server and make local code changes that will be
hot-reloaded in place.

Note: The development environement is created using tptdev tool.  The tptdev
tool references files in the source code so assumes, by default that it is being
run from the root of this repo.

Note: When running dev instances, the entrypoint process is air which
manages the live reload of code changes on your filesystem.  Therefore, if an
error occurs, the container will not fail and restart.  For example, if the build
fails due to a compile error for a live reload the container status will remain
`Running` because air is still running.  View the pod logs with `kubectl logs`
to see if this is the case.

You can call the API in one of two ways:

1. Call the API at the local IP exposed by a service type loadbalancer:
```bash
export API_IP=$(kubectl get svc -n threeport-control-plane threeport-api-server -o=jsonpath='{.status.loadBalancer.ingress[0].ip}')
curl $API_IP/swagger/index.html
```

2. Port forward localhost:1323 to the API:
```bash
make dev-forward-api
curl localhost:1323/swagger/index.html
```

Delete a local dev instance:

```bash
make dev-down
```

