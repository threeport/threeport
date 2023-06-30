# pkg/agent

This package contains the Kubernetes API extension for the threeport-agent.  We
use Kubernetes version conventions for this resource, currently `v1alpha1`.


The ThreeportWorkload API type is defined in
`api/v1alpha1/threeportworkload_types`.  If you need to make changes to the
fields available in that resource, that is where to do it.

This is an example of a ThreeportWorkload manifest in yaml:

```yaml
apiVersion: control-plane.threeport.io/v1alpha1
kind: ThreeportWorkload
metadata:
  creationTimestamp: "2023-06-27T18:05:44Z"
  finalizers:
  - control-plane.threeport.io/threeport-workload-finalizer
  generation: 1
  name: workload-instance-877588458330128385
  resourceVersion: "968"
  uid: 1c13ea8d-3753-4e7b-98ae-7f8d4828b1d6
spec:
  workloadInstanceId: 877588458330128385
  workloadResourceInstances:
  - kind: Namespace
    name: go-web3-sample-app-0
    threeportID: 877588461944930305
    version: v1
  - kind: ConfigMap
    name: go-web3-sample-app-config
    namespace: go-web3-sample-app-0
    threeportID: 877588461973536769
    version: v1
  - group: apps
    kind: Deployment
    name: go-web3-sample-app
    namespace: go-web3-sample-app-0
    threeportID: 877588462010892289
    version: v1
  - kind: Service
    name: go-web3-sample-app
    namespace: go-web3-sample-app-0
    threeportID: 877588462042742785
    version: v1
```

The `.spec.workloadInstanceID` field provides the workload instance ID which is
used by the threeport-agent when making updates in the Threeport API.  The
`.spec.workloadResourceInstances` field contains an array of all worklod
resource instances that constitute the workload instance.  The `threeportID` is
the workload resource instance ID in the Threeeport API and is
used when making updates to the Threeport API.  The `group`, `version`, `kind`,
`name` and `namespace` fields allow the threeport-agent to find the resource in
the Kubernetes API for watching.

The `api/v1alpha1/dynamic.go` source file continas a converter to convert
ThreeportWorkload resources into unstructured.Unstructured objects for use with
a dynamic client - used by the workload controller.

