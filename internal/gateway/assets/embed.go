package assets

import _ "embed"

//go:embed ingress-component.yaml
var IngressComponent []byte

//go:embed global-configuration.yaml
var GlobalConfiguration []byte

//go:embed nginx-tcp-service.yaml
var NginxTcpService []byte

//go:embed external-dns.yaml
var ExternalDNS []byte

//go:embed certificates.yaml
var Certificates []byte

//go:embed ingress.yaml
var Ingress []byte

//go:embed transport-server.yaml
var TransportServer []byte

//go:embed issuer.yaml
var Issuer []byte

//go:embed certificate.yaml
var Certificate []byte
