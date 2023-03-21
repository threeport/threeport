# Service Dependencies

## Libraries

### Repo: threeport/threeport-go-client
### Branch: service-dependencies

Fetch to local machine

### Repo: threeport/threeport-controller-library
### Branch: api-versioning

Fetch to local machine

## Core Control Plane Setup

### Repo: threeport/threeport-rest-api
### Branch: service-dependencies

Spin up dev environment for API Server:

```bash
make dev-up
```

Once dev environment is up, port forward the API and NATS server:

```bash
make dev-forward-api
make dev-forward-nats
```

## Run Workload Controller

### Repo: threeport/threeport-workload-controller
### Branch: service-dependencies

Run the workload controller locally:

```bash
make run
```

## Forward Proxy Operator

### Repo: qleet/forward-proxy-operator
### Branch: main

Install cert manager:

```bash
kubectl apply -f config/cert-manager/cert-manager.crds.yaml
kubectl apply -f config/cert-manager/cert-manager.yaml
```

Install envoy and TLS certs:

```bash
kubectl apply -f config/envoy/envoy.yaml
kubectl apply -f config/cert-manager/self-signed-ca.yaml
kubectl apply -f config/cert-manager/forward-proxy-cert.yaml
```

```bash
#export IMG=lander2k2/forward-proxy-operator:latest
#make deploy
```

## API Objects

Postman: AddWorkloadCluster

Get cluster attributes as follows from threeport-rest-api repo:

```bash
go run dev/workload-cluster-config.go -api-endpoint | xclip -sel clip  # APIEndpoint
go run dev/workload-cluster-config.go -ca-cert | xclip -sel clip  # CACertificate
go run dev/workload-cluster-config.go -cert | xclip -sel clip  # Certificate
go run dev/workload-cluster-config.go -key | xclip -sel clip  # Key
```

Postman: AddWorkloadDefinition

Postman: AddWorkloadInstance

Postman: AddWorkloadServiceDependency

TODO: Update workload service dependency

TODO: Use CLI instead of Postman

TODO: Changes when controller is not running (persistence in NATS)

## Sample App

Forward port:

```bash
kubectl -n sample-app port-forward svc/web3-sample-app 8080:8080
```

In private browser window visit: http://localhost:8080/

