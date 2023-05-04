package provider

import (
	"fmt"
	"path/filepath"

	"github.com/threeport/threeport/internal/kube"
)

// ControlPlaneInfra is the interface each provider has to satisfy to manage
// control plane infra.
type ControlPlaneInfra interface {
	Create(providerConfigDir string) (*kube.KubeConnectionInfo, error)
	Delete(providerConfigDir string) error
}

// ThreeportClusterName returns a name to use for a Kubernetes cluster where the
// threeport control plane runs.
func ThreeportClusterName(threeportInstanceName string) string {
	return fmt.Sprintf("threeport-%s", threeportInstanceName)
}

// ThreeportKubeconfigFilepath provides the filepath to the threeport-managed
// kubeconfig that is referenced by threeport tools when managing resources in
// Kubernetes.  This is distinct from the user's kubeconfig they use to connect
// to Kubernetes.
func ThreeportKubeconfigFilepath(path, instanceName string) string {
	filename := fmt.Sprintf("kubeconfig-%s", instanceName)
	return filepath.Join(path, filename)
}
