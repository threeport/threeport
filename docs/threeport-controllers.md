# Threeport Controllers

Threeport controllers provide the features and functionality of the threeport
control plane.  They are triggered by the API and store persistent data through
the API.  They work in isolation on the Threeport objects they are responsible
for reconciling.  They can trigger reconcilation in other controllers through
updates to the API of objects, and can be triggered by other controllers - or
user changes - to reconcile the state of objects they're responsible for.

## Controller Fundamentals

When changes are made to an object that requires reconciliation in the system,
a reconciler within a controller is notified by the API through the NATS
Jetstream messaging system.  It then works on a non-terminating loop to bring
about the desired state until that state is achieved.

It uses NATS to create distributed locks on particular objects while reconciling
so no other controller attempts to reconcile the same object simultaneously.  If
it encounters a condition that prevents reconcilation from completing, it
requeues the notification for reconciliation so that it is retried later.  This
requeue uses a backoff mechanism to progressively extend the interval between
reconciliation attempts to provide the best balance of responsiveness and
resource consumption.

### Controllers

A controller is a piece of software that runs as a part of the Threeport control
plane.

Controllers are based on the API data model.  A controller's domain of operation
is scoped by a source file in`pkg/api`.  For example the
kubernetes-runtime-controller is responsible for reconciling objects defined in
`pkg/api/v0/kubernetes_runtime.go`.

### Reconcilers

A controller consists of one or more reconcilers.  Each reconciler is
responsible for reconciling state for a single object.  For example, the
workload-controller has two reconcilers:

* Workload Definition Reconciler:  It is responsible for reconciling state for
  `WorkloadDefinition` objects.  It parses the `YAMLDocument` into separate
  Kubernetes resources and stores them each in a distinct
  `WorkloadResourceDefinition` object.
* Workload Instance Reconciler:  It is responsible for reconciling state for
  `WorkloadInstance` objects.  It takes all the `WorkloadResourceDefinitions`
  and installs them in a target Kubernetes cluster.

## Creating a New Controller

The following steps outline creating a new controller.  Examples are used for
the kubernetes-runtime-controller.  Refer to the code for that controller and
its objects for examples.

1. Create a data model for the objects that will be used and reconciled.
   Example: `pkg/api/v0/kubernetes_runtime.go`.
1. Add the following generate marker to the top of the source file:
   ```go
   //go:generate threeport-sdk gen controller --filename $GOFILE
   ```
1. Add the reconciler marker to those objects that will require reconcilation:
   ```go
   // +threeport-sdk:reconciler
   ```
   See the `KubernetesRuntimeDefinition` and `KubernetesRuntimeInstance` objects in
   `pkg/api/v0/kubernetes_runtime.go`.
   Note: not all objects necessarily require reconciliation.  Some just store
   data that is referred to when reconciling state for other objects.
1. If you have any "Definition" or "Instance objects that are getting a
   reconciler, you will need to include a `Reconciled` field of type `*bool`.
   The generated code will expect this.  This is not required if no reconciler
   exists for the object.
1. Create the following directories based on the name of the source file in the
   API.  For example:
   * `cmd/kubernetes-runtime-controller`
   * `internal/kubernetes-runtime`
1. Run code generation:
   ```bash
   make generate
   ```
1. You will find a new files in `internal/kubernetes-runtime` for each object being
   reconciled.  This example has two objects with a reconciler marker that get
   corresponding reconciler files:
   * `KubernetesRuntimeDefinition`: `internal/kubernetes-runtime/kubernetes_runtime_definition_gen.go`
   * `KubernetesRuntimeInstance`: `internal/kubernetes-runtime/kubernetes_runtime_instance_gen.go`
   In each reconciler file, you will find calls to some  as-yet-undefined
   functions.  In `internal/kubernetes-runtime/kubernetes_runtime_definition_gen.go`
   is:
   * `kubernetesRuntimeDefinitionCreated`
   * `kubernetesRuntimeDefinitionUpdate`
   * `kubernetesRuntimeDefinitionDelete`
1. Create a new file called `internal/kubernetes-runtime/kubernetes_runtime_definition.go`
   and add each of those functions
   with the business logic to reconcile the system when each of those actions
   occur, i.e. when a kubernetes runtime definition is created, update or deleted.
   The empty functions in
   `internal/kubernetes-runtime/kubernetes_runtime_definition.go` will look as
   follows.
   ```go
    // kubernetesRuntimeDefinitionCreated reconciles state for a new kubernetes
    // runtime definition.
    func kubernetesRuntimeDefinitionCreated(
        r *controller.Reconciler,
        kubernetesRuntimeDefinition *v0.KubernetesRuntimeDefinition,
        log *logr.Logger,
    ) error {
        return nil
    }

    // kubernetesRuntimeDefinitionCreated reconciles state for a kubernetes
    // runtime definition whenever it is changed.
    func kubernetesRuntimeDefinitionUpdated(
        r *controller.Reconciler,
        kubernetesRuntimeDefinition *v0.KubernetesRuntimeDefinition,
        log *logr.Logger,
    ) error {
        return nil
    }

    // kubernetesRuntimeDefinitionCreated reconciles state for a kubernetes
    // runtime definition whenever it is removed.
    func kubernetesRuntimeDefinitionDeleted(
        r *controller.Reconciler,
        kubernetesRuntimeDefinition *v0.KubernetesRuntimeDefinition,
        log *logr.Logger,
    ) error {
        return nil
    }
   ```
   Repeat for each reconciler.
1. Manually update the REST API main package where the NATS streams for
   controller notifications is added.  Look for the calls to `js.AddStream()`
   for other controllers and add the stream name and subjects for the new
   controller.  Follow the same naming pattern - the necessary constants will
   in the API package, e.g. `pkg/api/v0/kubernetes_runtime_gen.go`.
1. Create an image directory to build container images for the new controller.
   For now, copy an existing controllers Dockerfiles and modify to suit the new
   controller.
   Example:
   ```bash
   cp -R cmd/workload-controller/image cmd/kubernetes-runtime-controller
   ```

