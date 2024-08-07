# Gateways

Gateways provide a common support service to workloads.  They provide network
ingress into the Kubernetes Runtime to route traffic to a workload that is
exposed to end users, usually from the public internet.

When you declare a Gateway for your workload, Threeport installs and configures
a [Gloo Edge](https://docs.solo.io/gloo-edge/latest/) to manage incoming traffic.  A
cloud provider load balancer is also provisioned that provides a network
endpoint and proxies traffic to Gloo.  Gloo terminates TLS connections and
forwards connections to the appropriate workloads.

TLS assets are provisioned and rotated by
[cert-manager](https://cert-manager.io/).  Again, this support service is
installed and configured for the workload at runtime as needed.

## Gateway Definition

The gateway definition allows you define the HTTP and TCP ports you wish to use,
as well as the subdomain for a hosted zone if DNS records are also being
managed.  You can also provide the Kubernetes Service name that Gloo will
forward traffic to.  This will need to correspond to the Service resource name
in the Kubernetes resource manifest supplied with a Workload Definition.

You can also instruct Threeport to enable TLS - in which case cert-manager will
provision and rotate certs for your app.  You can also request HTTPS redirects
so that HTTP requests on port 80 will be redirected to HTTPS on 443.

You can also specify a request path to instruct the gateway to forward traffic
to different workloads based on the path in the request URL.

Reference:
[GatewayDefinition](https://pkg.go.dev/github.com/threeport/threeport/pkg/api/v0#GatewayDefinition)

## Gateway Instance

The gateway instance allows you to tie the gateway config in the definition to a
particular workload that prompts Threeport to deploy the Gloo Edge support
service and configure it for the workload.

Reference:
[GatewayInstance](https://pkg.go.dev/github.com/threeport/threeport/pkg/api/v0#GatewayInstance)

Domain names can be managed through a Threeport support service as well.
Threeport uses a project called
[external-dns](https://github.com/kubernetes-sigs/external-dns) to do this.
When using domain names, Threeport will install and configure external-dns as
needed.

## Domain Name Definition

The domain name definition allows you to configure a Route53 zone to use for DNS
records for your application.  For example if you have a hosted zone `myorg.com`
that manages DNS records for that domain, you can provide that in a definition
and a subdomain such as `myapp` in the gateway definition so that your app can
be reached at `myapp.myorg.com`.

Reference:
[DomainNameDefinition](https://pkg.go.dev/github.com/threeport/threeport/pkg/api/v0#DomainNameDefinition)

## Domain Name Instance

The domain name instance ties a workload to the domain name definition and
configures the external-dns support service to update Route53 to provide the
full domain name used by the workload.

Reference:
[DomainNameInstance](https://pkg.go.dev/github.com/threeport/threeport/pkg/api/v0#DomainNameInstance)

## Next Steps

See our [Deploy Workload on AWS guide](../workloads/deploy-workload-aws.md) for
an example of how to use Gateways and Domain Names for your application.

