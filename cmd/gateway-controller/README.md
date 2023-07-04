# Threeport gateway Controller

Manage gateways on Kubernetes clusters.

Here you will find the main package for the threeport gateway controller.  It
is responsible for reconciling changes made to GatewayDefinition and
GatewayInstance objects in the API.  In response, it manages gateways on the
Kubernetes cluster where it is running.

