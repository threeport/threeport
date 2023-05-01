package provider

import (
	"fmt"
	"time"

	"sigs.k8s.io/kind/pkg/apis/config/v1alpha4"
	"sigs.k8s.io/kind/pkg/cluster"
	"sigs.k8s.io/kind/pkg/cmd"

	"github.com/threeport/threeport/internal/kube"
)

type ControlPlaneInfraKind struct {
	ThreeportInstanceName string
	KubeconfigPath        string
	KindConfig            *v1alpha4.Cluster
	ThreeportPath         string
}

// Create installs a Kubernetes cluster using kind for the threeport control
// plane.
func (i *ControlPlaneInfraKind) Create() (*kube.KubeConnectionInfo, error) {
	logger := cmd.NewLogger()
	prov := cluster.NewProvider(
		cluster.ProviderWithLogger(logger),
	)

	// create the kind cluster
	if err := prov.Create(
		ThreeportClusterName(i.ThreeportInstanceName),
		cluster.CreateWithKubeconfigPath(i.KubeconfigPath),
		cluster.CreateWithWaitForReady(time.Duration(1000000000*60*5)), // 5 minutes
		cluster.CreateWithV1Alpha4Config(i.KindConfig),
	); err != nil {
		return &kube.KubeConnectionInfo{}, fmt.Errorf("failed to create new kind cluster: %w", err)
	}

	// get connection info from kubeconfig written by kind
	kubeConnInfo, err := kube.GetConnectionInfoFromKubeconfig(i.KubeconfigPath)
	if err != nil {
		return &kube.KubeConnectionInfo{}, fmt.Errorf("failed to get connection info from kubeconfig: %w", err)
	}

	return kubeConnInfo, nil
}

// Delete deletes a kind cluster and the threeport control plane with it.
func (i *ControlPlaneInfraKind) Delete() error {
	logger := cmd.NewLogger()
	prov := cluster.NewProvider(
		cluster.ProviderWithLogger(logger),
	)

	if err := prov.Delete(ThreeportClusterName(i.ThreeportInstanceName), i.KubeconfigPath); err != nil {
		return fmt.Errorf("failed to delete kind cluster: %w", err)
	}

	return nil
}

// GetKindConfig returns a kind config for users of threeport.
func (i *ControlPlaneInfraKind) GetKindConfig(devEnvironment bool, numWorkerNodes int) *v1alpha4.Cluster {
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
				ExtraMounts: []v1alpha4.Mount{
					{
						ContainerPath: "/threeport",
						HostPath:      i.ThreeportPath,
					},
				},
			},
		},
	}

	var workerNodes *[]v1alpha4.Node
	if devEnvironment {
		workerNodes = devEnvKindWorkers(i.ThreeportPath, numWorkerNodes)
	} else {
		workerNodes = kindWorkers(numWorkerNodes)
	}
	for _, n := range *workerNodes {
		clusterConfig.Nodes = append(clusterConfig.Nodes, n)
	}

	return &clusterConfig
}

// devEnvKindWorkers returns worker nodes with host path mount for live code
// reloads.
func devEnvKindWorkers(threeportPath string, numWorkerNodes int) *[]v1alpha4.Node {
	nodes := make([]v1alpha4.Node, numWorkerNodes)
	for i := range nodes {
		nodes[i] = v1alpha4.Node{
			Role: v1alpha4.WorkerRole,
			ExtraMounts: []v1alpha4.Mount{
				{
					ContainerPath: "/threeport",
					HostPath:      threeportPath,
				},
			},
		}
	}
	return &nodes
}

// kindWorkers returns regular worker nodes
func kindWorkers(numWorkerNodes int) *[]v1alpha4.Node {
	nodes := make([]v1alpha4.Node, numWorkerNodes)
	for i := range nodes {

		nodes[i] = v1alpha4.Node{
			Role: v1alpha4.WorkerRole,
		}

	}
	return &nodes
}
