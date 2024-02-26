package controller

type ControllerConfig struct {
	// The name of the controller in kebab case, e.g.
	// kubernetes-runtime-controller
	Name string

	// The name of the controller in kebab case sans "-controler", e.g
	// kubernetes-runtime
	ShortName string

	// The name of the controller in lower case, no spaces, e.g.
	// kubernetesruntime
	PackageName string

	// The name of a NATS Jetstream stream for a controller, e.g.
	// KubernetesRuntimeStreamName
	StreamName string

	// The objects for which reconcilers should be generated.
	ReconciledObjects []string
}
