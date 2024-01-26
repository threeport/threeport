# Threeport Agent

The Threeport Agent is a Kubernetes Operator that runs in all Threeport
Kubernetes runtime instances.  It's job is to watch Threeport-deployed workloads
and report back status events to the Threeport API for users to consume.

The Threeport Agent is *not* intended as a full-featured observability
soluation, but rather a high-level indication of whether systems are in working
order.  Any errors or events that can be derived from the Kubernetes API are
in-scope for the Threeport Agent.  Observability systems should be used for
deeper logging and metrics data.

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

## See Also

* [threeport-agent README](../cmd/agent/README.md)

