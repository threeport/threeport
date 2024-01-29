# Threeport Agent

The Threeport Agent is a Kubernetes Operator that runs in all Threeport
Kubernetes runtime instances.  It's job is to watch Threeport-deployed workloads
and report back status events to the Threeport API for users to consume.

The Threeport Agent is *not* intended as a full-featured observability
soluation, but rather a high-level indication of whether systems are in working
order.  Any errors or events that can be derived from the Kubernetes API are
in-scope for the Threeport Agent.  Observability systems should be used for
deeper logging and metrics data.

## See Also

* [threeport-agent README](../cmd/agent/README.md)

## Interface with Kubernetes

When a workload instance controller creates resources in Kubernetes, it uses a
custom `ThreeportWorkload` resource created in the same cluster to inform the
Threeport Agent of Kubernetes resources it sould watch and report upon.

The Threeport Agent watches `ThreeportWorkload` resources and places new watches
based on the information found therein.

The `ThreeportWorkload` resource is defined in
`pkg/agent/api/[version]/threeportworkload_types.go`.

When any change is made to the fields of the `ThreeportAgent` resource the
corresponding change will need to be made to the definition of the
`threeportworkloads.control-plane.threeport.io` CRD in
`pkg/threeport-installer/v0/components.go`.  We currently do not have an
automated way for generating this but the `controller-gen` utility used by the
[kubebuilder project](https://github.com/kubernetes-sigs/kubebuilder) could be
leveraged.

## Reporting to Threeport

When events occur related to watched Kubernetes workload resources, it makes
calls back to the Threeport API to surface those events to Threeport users.

## Threeport Internals

The Threeport Agent watches and reports on all resources that are deployed as a
part of a workload.

The following diagram illustrates what the Threeport Agent does internally.
This example shows what it does to report a Deployment resource in Kubernetes.
In the case of a Deployment, the Pods and Replicasets are also watched and
events collected.  The same goes for other Kubernetes resources that abstract
Pods.  For all other Kubernetes resources those abstracted types aren't
relevant.

The agent watches the Deployment and reports all operations that are made on it.
It also starts an informer to collect events that are related to that
Deployment.  Events related to the derived Repliaset and Pods is also reported.

Everything is sent on channels to a Notifer that is responsible for sending
updates to the Threeport API.

![threeport-agent internals](../../docs/img/ThreeportAgentInternals.png)

