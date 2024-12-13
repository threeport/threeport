# Threeport Gateway Controller

Manage gateways on Kubernetes clusters.

Here you will find the main package for the Threeport gateway controller.  It
is responsible for reconciling changes made to GatewayDefinition,
GatewayInstance, DomainNameDefinition and DomainNameInstance objects in the API.
In response, it manages gateways and DNS records on the Kubernetes cluster where
it is running.

It leverages the
[support-services-operator](https://github.com/nukleros/support-services-operator)
to manage resources in the Kubernetes clusters under management.

