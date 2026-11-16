package v0

// ContainerResources is the CPU and memory a control plane container requests
// and is limited to, as Kubernetes quantities, e.g. "100m" or "256Mi".
type ContainerResources struct {
	RequestCPU    string
	RequestMemory string
	LimitCPU      string
	LimitMemory   string
}

// manifest returns the resources block of a container spec.
func (r ContainerResources) manifest() map[string]interface{} {
	return map[string]interface{}{
		"requests": map[string]interface{}{
			"cpu":    r.RequestCPU,
			"memory": r.RequestMemory,
		},
		"limits": map[string]interface{}{
			"cpu":    r.LimitCPU,
			"memory": r.LimitMemory,
		},
	}
}

// DefaultComponentResources returns the resources a control plane component's
// container gets unless Options.ComponentResources overrides them.
//
// Measured on a development control plane under the integration suite, every
// component stayed under 35m CPU and 50Mi memory. Requests sit near that, so a
// small cluster can schedule the whole control plane, and limits leave room for
// load the suite does not generate. Controllers that run infrastructure tools
// get far more memory than the suite ever asked of them: the terraform
// controller runs the terraform binary and its providers inside its own
// container, the aws, oci and gcp controllers run pulumi and its provider
// plugins the same way, and the helm workload controller renders charts in
// process.
func DefaultComponentResources(name string) ContainerResources {
	switch name {
	case ThreeportRestApiName:
		return ContainerResources{RequestCPU: "50m", RequestMemory: "64Mi", LimitCPU: "1", LimitMemory: "512Mi"}
	case ThreeportAgentName:
		return ContainerResources{RequestCPU: "10m", RequestMemory: "64Mi", LimitCPU: "500m", LimitMemory: "256Mi"}
	case ThreeportTerraformControllerName,
		ThreeportAwsControllerName,
		ThreeportOciControllerName,
		ThreeportGcpControllerName:
		return ContainerResources{RequestCPU: "50m", RequestMemory: "128Mi", LimitCPU: "1", LimitMemory: "2Gi"}
	case ThreeportHelmWorkloadControllerName:
		return ContainerResources{RequestCPU: "20m", RequestMemory: "64Mi", LimitCPU: "1", LimitMemory: "1Gi"}
	default:
		return ContainerResources{RequestCPU: "10m", RequestMemory: "32Mi", LimitCPU: "500m", LimitMemory: "256Mi"}
	}
}

// componentResources returns the resources block for a component's container:
// its defaults, with any value set in Options.ComponentResources for it in
// place of the default.  An override can set one value and keep the rest, so
// a field it leaves empty is never written as an empty quantity.
func (cpi *ControlPlaneInstaller) componentResources(name string) map[string]interface{} {
	resources := DefaultComponentResources(name)

	override := cpi.Opts.ComponentResources[name]
	if override.RequestCPU != "" {
		resources.RequestCPU = override.RequestCPU
	}
	if override.RequestMemory != "" {
		resources.RequestMemory = override.RequestMemory
	}
	if override.LimitCPU != "" {
		resources.LimitCPU = override.LimitCPU
	}
	if override.LimitMemory != "" {
		resources.LimitMemory = override.LimitMemory
	}

	return resources.manifest()
}
