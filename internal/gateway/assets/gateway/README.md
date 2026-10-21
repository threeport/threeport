# Gateway Definition Manifests

`createGatewayDefinitionYamlDocument()` renders these manifests for one
gateway definition.

`ingress.yaml` is an Ingress for the HTTP ports.

`transport-server.yaml` is a TransportServer for each TCP port.

`issuer.yaml` is an Issuer and `certificate.yaml` is a Certificate.  Both
are included when any port on the definition has TLS enabled.
