# Runtime TCP Manifests

`confirmGatewayPortsExposed()` keeps one GlobalConfiguration for a
Kubernetes runtime.  It rewrites its listeners from the TCP ports of every
gateway on the runtime.

`global-configuration.yaml` is a GlobalConfiguration named `nginx-ingress`.
Its listeners are those ports.
