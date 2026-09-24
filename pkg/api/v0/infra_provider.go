package v0

import (
	"fmt"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// Runtime identifies which runtime domain a cloud provider is being
// resolved for. A single cloud provider can back multiple domains
// (GCP → GKE for kubernetes runtime, GCE for machine runtime).
type Runtime string

const (
	RuntimeKubernetes Runtime = "kubernetes-runtime"
	RuntimeMachine    Runtime = "machine-runtime"
)

// InfraProviderForCloudProvider maps a cloud provider name to the concrete
// infra provider token for the given runtime domain. Cloud provider names
// are what users write in user-facing config (RouterMachine, Fleet,
// RouterMachineSet); the domain plus cloud resolves to a single infra
// provider token that the corresponding controller keys its provisioning
// switch on. Returns an error for an empty cloud provider or a cloud
// provider unsupported for the domain so an accidental typo surfaces at
// the caller.
func InfraProviderForCloudProvider(cloudProvider string, runtime Runtime) (string, error) {
	if cloudProvider == "" {
		return "", fmt.Errorf("cloud provider is required")
	}
	switch runtime {
	case RuntimeKubernetes:
		switch cloudProvider {
		case util.GcpProvider:
			return KubernetesRuntimeInfraProviderGKE, nil
		case util.AwsProvider:
			return KubernetesRuntimeInfraProviderEKS, nil
		case util.OciProvider:
			return KubernetesRuntimeInfraProviderOKE, nil
		}
	case RuntimeMachine:
		switch cloudProvider {
		case util.GcpProvider:
			return MachineRuntimeInfraProviderGCE, nil
		}
	default:
		return "", fmt.Errorf("runtime %q is not supported", runtime)
	}
	return "", fmt.Errorf("cloud provider %q is not supported for runtime %q", cloudProvider, runtime)
}

// CloudProviderForInfraProvider maps an infra provider token back to its
// cloud provider name. Handles infra tokens from every runtime domain;
// each infra token uniquely identifies a cloud so a runtime hint is not
// needed.
func CloudProviderForInfraProvider(infraProvider string) (string, error) {
	switch infraProvider {
	case KubernetesRuntimeInfraProviderGKE, MachineRuntimeInfraProviderGCE:
		return util.GcpProvider, nil
	case KubernetesRuntimeInfraProviderEKS:
		return util.AwsProvider, nil
	case KubernetesRuntimeInfraProviderOKE:
		return util.OciProvider, nil
	case KubernetesRuntimeInfraProviderKind:
		return util.AwsProvider, nil
	case "":
		return "", fmt.Errorf("infra provider is required")
	}
	return "", fmt.Errorf("infra provider %q is not supported", infraProvider)
}
