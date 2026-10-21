# Support-services Operator Requests

Threeport applies these custom resources once for a Kubernetes runtime.
The operator creates the controllers from them.

`ingress-component.yaml` is an IngressComponent and installs nginx.
`getExternalDnsYaml()` uses the same file for a domain name and sets
`externalDNS`.

`certificates.yaml` is a CertificatesComponent and installs cert-manager.
`confirmGatewayControllerDeployed()` includes it for EKS and kind.
