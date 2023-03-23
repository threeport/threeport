package install

import "fmt"

const (
	FowardProxyOperatorImage = "lander2k2/forward-proxy-operator:v0.0.4"
)

// ForwardProxyManifest returns a yaml manifest for the forward proxy operator
// and a ForwardProxyServer manifest to spin up the envoy forward proxy
// instance.
// https://github.com/qleet/forward-proxy-operator
func ForwardProxyManifest() string {
	return fmt.Sprintf(`---
apiVersion: v1
kind: Namespace
metadata:
  labels:
    app.kubernetes.io/component: manager
    app.kubernetes.io/created-by: forward-proxy-operator
    app.kubernetes.io/instance: system
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/name: namespace
    app.kubernetes.io/part-of: forward-proxy-operator
    control-plane: controller-manager
  name: forward-proxy-system
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  annotations:
    controller-gen.kubebuilder.io/version: v0.9.0
  creationTimestamp: null
  name: forwardproxies.routing.qleet.io
spec:
  group: routing.qleet.io
  names:
    kind: ForwardProxy
    listKind: ForwardProxyList
    plural: forwardproxies
    singular: forwardproxy
  scope: Namespaced
  versions:
  - name: v1alpha1
    schema:
      openAPIV3Schema:
        description: ForwardProxy is the Schema for the forwardproxies API
        properties:
          apiVersion:
            description: 'APIVersion defines the versioned schema of this representation
              of an object. Servers should convert recognized schemas to the latest
              internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources'
            type: string
          kind:
            description: 'Kind is a string value representing the REST resource this
              object represents. Servers may infer this from the endpoint the client
              submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds'
            type: string
          metadata:
            type: object
          spec:
            description: ForwardProxySpec defines the desired state of ForwardProxy
            properties:
              upstreamHost:
                description: UpstreamHost is the destination hostname
                type: string
              upstreamPath:
                description: UpstreamPath is the path to the intended resource at
                  the destination
                type: string
            type: object
          status:
            description: ForwardProxyStatus defines the observed state of ForwardProxy
            type: object
        type: object
    served: true
    storage: true
    subresources:
      status: {}
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  annotations:
    controller-gen.kubebuilder.io/version: v0.9.0
  creationTimestamp: null
  name: forwardproxyservers.routing.qleet.io
spec:
  group: routing.qleet.io
  names:
    kind: ForwardProxyServer
    listKind: ForwardProxyServerList
    plural: forwardproxyservers
    singular: forwardproxyserver
  scope: Cluster
  versions:
  - name: v1alpha1
    schema:
      openAPIV3Schema:
        description: ForwardProxyServer is the Schema for the forwardproxyservers
          API.
        properties:
          apiVersion:
            description: 'APIVersion defines the versioned schema of this representation
              of an object. Servers should convert recognized schemas to the latest
              internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources'
            type: string
          kind:
            description: 'Kind is a string value representing the REST resource this
              object represents. Servers may infer this from the endpoint the client
              submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds'
            type: string
          metadata:
            type: object
          spec:
            description: ForwardProxyServerSpec defines the desired state of ForwardProxyServer.
            properties:
              namespace:
                default: forward-proxy-system
                description: '(Default: "forward-proxy-system") Namespace to use for
                  ingress support services.'
                type: string
              replicas:
                default: 2
                description: '(Default: 2) Number of replicas to use for the forward
                  proxy server.'
                type: integer
            type: object
          status:
            description: ForwardProxyServerStatus defines the observed state of ForwardProxyServer.
            properties:
              conditions:
                items:
                  description: PhaseCondition describes an event that has occurred
                    during a phase of the controller reconciliation loop.
                  properties:
                    lastModified:
                      description: LastModified defines the time in which this component
                        was updated.
                      type: string
                    message:
                      description: Message defines a helpful message from the phase.
                      type: string
                    phase:
                      description: Phase defines the phase in which the condition
                        was set.
                      type: string
                    state:
                      description: PhaseState defines the current state of the phase.
                      enum:
                      - Complete
                      - Reconciling
                      - Failed
                      - Pending
                      type: string
                  required:
                  - lastModified
                  - message
                  - phase
                  - state
                  type: object
                type: array
              created:
                type: boolean
              dependenciesSatisfied:
                type: boolean
              resources:
                items:
                  description: ChildResource is the resource and its condition as
                    stored on the workload custom resource's status field.
                  properties:
                    condition:
                      description: ResourceCondition defines the current condition
                        of this resource.
                      properties:
                        created:
                          description: Created defines whether this object has been
                            successfully created or not.
                          type: boolean
                        lastModified:
                          description: LastModified defines the time in which this
                            resource was updated.
                          type: string
                        message:
                          description: Message defines a helpful message from the
                            resource phase.
                          type: string
                      required:
                      - created
                      type: object
                    group:
                      description: Group defines the API Group of the resource.
                      type: string
                    kind:
                      description: Kind defines the kind of the resource.
                      type: string
                    name:
                      description: Name defines the name of the resource from the
                        metadata.name field.
                      type: string
                    namespace:
                      description: Namespace defines the namespace in which this resource
                        exists in.
                      type: string
                    version:
                      description: Version defines the API Version of the resource.
                      type: string
                  required:
                  - group
                  - kind
                  - name
                  - namespace
                  - version
                  type: object
                type: array
            type: object
        type: object
    served: true
    storage: true
    subresources:
      status: {}
---
apiVersion: v1
kind: ServiceAccount
metadata:
  labels:
    app.kuberentes.io/instance: controller-manager
    app.kubernetes.io/component: rbac
    app.kubernetes.io/created-by: forward-proxy-operator
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/name: serviceaccount
    app.kubernetes.io/part-of: forward-proxy-operator
  name: forward-proxy-controller-manager
  namespace: forward-proxy-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  labels:
    app.kubernetes.io/component: rbac
    app.kubernetes.io/created-by: forward-proxy-operator
    app.kubernetes.io/instance: leader-election-role
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/name: role
    app.kubernetes.io/part-of: forward-proxy-operator
  name: forward-proxy-leader-election-role
  namespace: forward-proxy-system
rules:
- apiGroups:
  - ""
  resources:
  - configmaps
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
- apiGroups:
  - coordination.k8s.io
  resources:
  - leases
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
- apiGroups:
  - ""
  resources:
  - events
  verbs:
  - create
  - patch
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  creationTimestamp: null
  name: forward-proxy-manager-role
rules:
- apiGroups:
  - apps
  resources:
  - deployments
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - ""
  resources:
  - configmaps
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - ""
  resources:
  - namespaces
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - ""
  resources:
  - services
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - routing.qleet.io
  resources:
  - forwardproxies
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - routing.qleet.io
  resources:
  - forwardproxies/finalizers
  verbs:
  - update
- apiGroups:
  - routing.qleet.io
  resources:
  - forwardproxies/status
  verbs:
  - get
  - patch
  - update
- apiGroups:
  - routing.qleet.io
  resources:
  - forwardproxyservers
  verbs:
  - create
  - delete
  - get
  - list
  - patch
  - update
  - watch
- apiGroups:
  - routing.qleet.io
  resources:
  - forwardproxyservers/status
  verbs:
  - get
  - patch
  - update
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  labels:
    app.kubernetes.io/component: kube-rbac-proxy
    app.kubernetes.io/created-by: forward-proxy-operator
    app.kubernetes.io/instance: metrics-reader
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/name: clusterrole
    app.kubernetes.io/part-of: forward-proxy-operator
  name: forward-proxy-metrics-reader
rules:
- nonResourceURLs:
  - /metrics
  verbs:
  - get
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  labels:
    app.kubernetes.io/component: kube-rbac-proxy
    app.kubernetes.io/created-by: forward-proxy-operator
    app.kubernetes.io/instance: proxy-role
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/name: clusterrole
    app.kubernetes.io/part-of: forward-proxy-operator
  name: forward-proxy-proxy-role
rules:
- apiGroups:
  - authentication.k8s.io
  resources:
  - tokenreviews
  verbs:
  - create
- apiGroups:
  - authorization.k8s.io
  resources:
  - subjectaccessreviews
  verbs:
  - create
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  labels:
    app.kubernetes.io/component: rbac
    app.kubernetes.io/created-by: forward-proxy-operator
    app.kubernetes.io/instance: leader-election-rolebinding
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/name: rolebinding
    app.kubernetes.io/part-of: forward-proxy-operator
  name: forward-proxy-leader-election-rolebinding
  namespace: forward-proxy-system
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: forward-proxy-leader-election-role
subjects:
- kind: ServiceAccount
  name: forward-proxy-controller-manager
  namespace: forward-proxy-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  labels:
    app.kubernetes.io/component: rbac
    app.kubernetes.io/created-by: forward-proxy-operator
    app.kubernetes.io/instance: manager-rolebinding
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/name: clusterrolebinding
    app.kubernetes.io/part-of: forward-proxy-operator
  name: forward-proxy-manager-rolebinding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: forward-proxy-manager-role
subjects:
- kind: ServiceAccount
  name: forward-proxy-controller-manager
  namespace: forward-proxy-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  labels:
    app.kubernetes.io/component: kube-rbac-proxy
    app.kubernetes.io/created-by: forward-proxy-operator
    app.kubernetes.io/instance: proxy-rolebinding
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/name: clusterrolebinding
    app.kubernetes.io/part-of: forward-proxy-operator
  name: forward-proxy-proxy-rolebinding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: forward-proxy-proxy-role
subjects:
- kind: ServiceAccount
  name: forward-proxy-controller-manager
  namespace: forward-proxy-system
---
apiVersion: v1
kind: Service
metadata:
  labels:
    app.kubernetes.io/component: kube-rbac-proxy
    app.kubernetes.io/created-by: forward-proxy-operator
    app.kubernetes.io/instance: controller-manager-metrics-service
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/name: service
    app.kubernetes.io/part-of: forward-proxy-operator
    control-plane: controller-manager
  name: forward-proxy-controller-manager-metrics-service
  namespace: forward-proxy-system
spec:
  ports:
  - name: https
    port: 8443
    protocol: TCP
    targetPort: https
  selector:
    control-plane: controller-manager
---
apiVersion: v1
kind: Service
metadata:
  name: forward-proxy-controller-manager-xds-service
  namespace: forward-proxy-system
spec:
  ports:
  - name: xds
    port: 18000
    protocol: TCP
    targetPort: 18000
  selector:
    control-plane: controller-manager
---
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app.kubernetes.io/component: manager
    app.kubernetes.io/created-by: forward-proxy-operator
    app.kubernetes.io/instance: controller-manager
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/name: deployment
    app.kubernetes.io/part-of: forward-proxy-operator
    control-plane: controller-manager
  name: forward-proxy-controller-manager
  namespace: forward-proxy-system
spec:
  replicas: 1
  selector:
    matchLabels:
      control-plane: controller-manager
  template:
    metadata:
      annotations:
        kubectl.kubernetes.io/default-container: manager
      labels:
        control-plane: controller-manager
    spec:
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
            - matchExpressions:
              - key: kubernetes.io/arch
                operator: In
                values:
                - amd64
                - arm64
                - ppc64le
                - s390x
              - key: kubernetes.io/os
                operator: In
                values:
                - linux
      containers:
      - args:
        - --secure-listen-address=0.0.0.0:8443
        - --upstream=http://127.0.0.1:8080/
        - --logtostderr=true
        - --v=0
        image: gcr.io/kubebuilder/kube-rbac-proxy:v0.13.0
        name: kube-rbac-proxy
        ports:
        - containerPort: 8443
          name: https
          protocol: TCP
        resources:
          limits:
            cpu: 500m
            memory: 128Mi
          requests:
            cpu: 5m
            memory: 64Mi
        securityContext:
          allowPrivilegeEscalation: false
          capabilities:
            drop:
            - ALL
      - args:
        - --health-probe-bind-address=:8081
        - --metrics-bind-address=127.0.0.1:8080
        - --leader-elect
        command:
        - /manager
        image: %[1]s
        imagePullPolicy: IfNotPresent
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8081
          initialDelaySeconds: 15
          periodSeconds: 20
        name: manager
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 10
        resources:
          limits:
            cpu: 500m
            memory: 128Mi
          requests:
            cpu: 10m
            memory: 64Mi
        securityContext:
          allowPrivilegeEscalation: false
          capabilities:
            drop:
            - ALL
      securityContext:
        runAsNonRoot: true
      serviceAccountName: forward-proxy-controller-manager
      terminationGracePeriodSeconds: 10
---
apiVersion: routing.qleet.io/v1alpha1
kind: ForwardProxyServer
metadata:
  name: foward-proxy-main
spec:
  namespace: "forward-proxy-system"
  replicas: 2
`, FowardProxyOperatorImage)
}
