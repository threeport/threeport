# Runtime TCP Manifests

`confirmGatewayPortsExposed()` keeps one copy of each manifest for a
Kubernetes runtime.  It rewrites that copy from the TCP ports of every
gateway on the runtime.

`global-configuration.yaml` is a GlobalConfiguration named `nginx-ingress`.
Its listeners are those ports.

`nginx-tcp-service.yaml` is a Service named `nginx-tcp`.  It publishes
those ports.

`nginx-tcp-controller.yaml` is a Deployment named `nginx-tcp`.  It runs
nginx with `-global-configuration` and ingress class `nginx-tcp`.
