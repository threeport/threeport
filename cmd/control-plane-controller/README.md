# Threeport Control Plane Controller

Manage other Threeport control planes with an existing Threeport control plane.

Here you will find the main package for the Threeport control plane controller.  It
is responsible for reconciling changes made to ControlPlaneDefinition and
ControlPlaneInstance objects in the API.

A control plane requires a reconciled Kubernetes runtime instance. That will be the infrastructure on which
the new control plane will be deployed on. Once deployed it will be considered as a child control plane to the
one that deploys it. The deploying control plane will thus be a parent. This helps maintain a topological tree-like relationship
between control planes being managed by one another. The root of this tree is considered the Genesis control plane.
