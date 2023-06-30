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
is scoped by a source file in`pkg/api`.  For example the workload-controller is
responsible for reconciling objects defined in `pkg/api/v0/workload.go`.

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
the workload-controller.  Refer to the code for that controller for examples.

1. Create a data model for the objects that will be used and reconciled.  Some
   objects simply store data and do not require reconcilation of state.  For
   example in `pkg/api/v0/workload.go`, the `WorkloadResourceDefinition` and
   `WorkloadResourceInstance` objects are not reconciled.  They simply store
   data and are a resource for reconciliation of other objects.
1. Add the following generate marker to the top of the source file:
   ```go
   //go:generate ../../../bin/threeport-codegen controller --filename $GOFILE
   ```
   See `pkg/api/v0/workload.go` for an example.
1. Add the reconciler marker to those objects that will require reconcilation:
   ```go
   // +threeport-codegen:reconciler
   ```
   See the `WorkloadDefinition` and `WorkloadInstance` objects in
   `pkg/api/v0/workload.go`.
1. Create the following directories based on the name of the source file in the
   API.  For example if you data models are defined in `pkg/api/v0/animal.go` you
   will need the following directories:
   * `cmd/animal-controller`
   * `internal/animal`
1. Run code generation:
   ```bash
   make generate
   ```
1. You will find a new files in `internal/animal` for each object being
   reconciled.  If you had `Cat` and `Dog` objects that had a reconciler marker
   on them, you will find two new files:
   * `internal/animal/cat_gen.go`
   * `internal/animal/dog_gen.go`
   In the case of the cat reconciler, you will find calls to some
   as-yet-undefined functions:
   * `catCreated`
   * `catDeleted`
1. Create a new file called `internal/animal/cat.go` and add those functions
   with the business logic to reconcile the system when each of those actions
   occur, i.e. when a cat is created or deleted.  Repeat for the dog reconciler.
1. Manually update the REST API main package where the NATS streams for
   controller notifications is added.  Look for the calls to `js.AddStream()`
   for other controllers and add the stream name and subjects for the new
   controller.  Follow the same naming pattern - the necessary constants will
   already have been generated in `pkg/api/v0/[controller name]_gen.go`.

