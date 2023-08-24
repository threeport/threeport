# Threeport Kubernetes Runtime Controller

Manage Kubernetes clusters as runtime environments for workloads.

Here you will find the main package for the threeport kubernetes runtime
controller.  It is responsible for reconciling KubernetesRuntimeDefinition and
KubernetesRuntimeInstance objects.  It serves as an abstraction to
cloud-provider specific implementation details so that users can declare a
runtime with a set of high-level configs.   This controller translates those
configs into cloud provider values and creates the provider-specific resource on
the user's behlaf.  It also contains connection information and credentials that
other controllers, such as the workload controller, use to connect to the
Kubernetes API to manage workloads.

