package kind

import (
	"fmt"
	"time"

	"sigs.k8s.io/kind/pkg/apis/config/v1alpha4"
	"sigs.k8s.io/kind/pkg/cluster"
	"sigs.k8s.io/kind/pkg/cmd"

	"github.com/threeport/threeport/internal/kube"
)

// CreateKindDevCluster creates a new kind cluster for a development
// environment.  The dev kind cluster includes configs to allow live reloading
// of code.
func CreateKindDevCluster(name, kubeconfig, threeportPath string) (*kube.KubeConnectionInfo, error) {
	logger := cmd.NewLogger()
	provider := cluster.NewProvider(
		cluster.ProviderWithLogger(logger),
	)

	// configure the kind cluster
	clusterConfig := v1alpha4.Cluster{
		Nodes: []v1alpha4.Node{
			{
				Role: v1alpha4.ControlPlaneRole,
				KubeadmConfigPatches: []string{
					`kind: InitConfiguration
nodeRegistration:
  kubeletExtraArgs:
    node-labels: "ingress-ready=true"
`,
				},
				ExtraPortMappings: []v1alpha4.PortMapping{
					{
						ContainerPort: int32(80),
						HostPort:      int32(80),
						Protocol:      v1alpha4.PortMappingProtocolTCP,
					},
					{
						ContainerPort: int32(443),
						HostPort:      int32(443),
						Protocol:      v1alpha4.PortMappingProtocolTCP,
					},
				},
			},
			{
				Role: v1alpha4.WorkerRole,
				ExtraMounts: []v1alpha4.Mount{
					{
						ContainerPath: "/threeport",
						HostPath:      threeportPath,
					},
				},
			},
			{
				Role: v1alpha4.WorkerRole,
				ExtraMounts: []v1alpha4.Mount{
					{
						ContainerPath: "/threeport",
						HostPath:      threeportPath,
					},
				},
			},
			{
				Role: v1alpha4.WorkerRole,
				ExtraMounts: []v1alpha4.Mount{
					{
						ContainerPath: "/threeport",
						HostPath:      threeportPath,
					},
				},
			},
		},
	}

	// create the kind cluster
	if err := provider.Create(
		name,
		cluster.CreateWithKubeconfigPath(kubeconfig),
		cluster.CreateWithWaitForReady(time.Duration(1000000000*60*5)), // 5 minutes
		cluster.CreateWithV1Alpha4Config(&clusterConfig),
	); err != nil {
		return &kube.KubeConnectionInfo{}, fmt.Errorf("failed to create new kind cluster: %w", err)
	}

	// get connection info from kubeconfig written by kind
	kubeConnInfo, err := kube.GetConnectionInfoFromKubeconfig(kubeconfig)
	if err != nil {
		return &kube.KubeConnectionInfo{}, fmt.Errorf("failed to get connection info from kubeconfig: %w", err)
	}

	return kubeConnInfo, nil
}

// DeleteKindCluster deletes a kind cluster.
func DeleteKindCluster(name, kubeconfig string) error {
	logger := cmd.NewLogger()
	provider := cluster.NewProvider(
		cluster.ProviderWithLogger(logger),
	)

	if err := provider.Delete(name, kubeconfig); err != nil {
		return fmt.Errorf("failed to delete kind cluster: %w", err)
	}

	return nil
}
