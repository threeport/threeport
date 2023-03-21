# Threeport

An application orchestration control plane.

## Quickstart

In order to run a local development instance of threeport, you'll need:

* [docker](https://docs.docker.com/get-docker/)
* [kind](https://kind.sigs.k8s.io/)
* [kubectl](https://kubernetes.io/docs/reference/kubectl/)

Spin up a local dev instance:

```bash
make dev-up
```

This will start a local kind cluster and install the control plane.  You can now
make calls to the API server and make local code changes that will be
hot-reloaded in place.

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

## Image Build

Build an image for REST API:
```bash
REST_API_IMG=threeport-rest-api:dev make rest-api-image
```

