package manifests

import _ "embed"

//go:embed sso/ingress-component.yaml
var IngressComponent []byte

//go:embed shared/global-configuration.yaml
var GlobalConfiguration []byte

//go:embed shared/nginx-tcp-service.yaml
var NginxTcpService []byte

//go:embed shared/nginx-tcp-controller.yaml
var NginxTcpController []byte

//go:embed sso/certificates.yaml
var Certificates []byte

//go:embed gateway/ingress.yaml
var Ingress []byte

//go:embed gateway/transport-server.yaml
var TransportServer []byte

//go:embed gateway/issuer.yaml
var Issuer []byte

//go:embed gateway/certificate.yaml
var Certificate []byte
