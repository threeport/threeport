package provider

import (
	"fmt"
	"time"

	"sigs.k8s.io/kind/pkg/apis/config/v1alpha4"
	"sigs.k8s.io/kind/pkg/cluster"
	"sigs.k8s.io/kind/pkg/cmd"

	"github.com/threeport/threeport/internal/kube"
)

// ClusterInfraKind represents a kind cluster for local a threeport instance.
type ClusterInfraKind struct {
	// The unique name of the threeport instance.
	ThreeportInstanceName string

	// Path to user's kubeconfig file for connecting to Kubernetes API.
	KubeconfigPath string

	// True if threeport instance is for a development environment with live
	// reloads of code from filesystem.
	DevEnvironment bool

	// Used only for development environments.  The path to the threeport repo
	// on the developer's file system.
	ThreeportPath string

	// Number of worker nodes for kind cluster.
	NumWorkerNodes int
}

// Create installs a Kubernetes cluster using kind for the threeport control
// plane.
func (i *ClusterInfraKind) Create() (*kube.KubeConnectionInfo, error) {
	logger := cmd.NewLogger()
	prov := cluster.NewProvider(
		cluster.ProviderWithLogger(logger),
	)

	// create the kind cluster
	if err := prov.Create(
		i.RuntimeInstanceName,
		cluster.CreateWithKubeconfigPath(i.KubeconfigPath),
		cluster.CreateWithWaitForReady(time.Duration(1000000000*60*5)), // 5 minutes
		cluster.CreateWithV1Alpha4Config(getKindConfig(i.DevEnvironment, i.ThreeportPath, i.NumWorkerNodes)),
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
// func (i *ClusterInfraKind) Delete(providerConfigDir string) error {
func (i *ClusterInfraKind) Delete() error {
	logger := cmd.NewLogger()
	prov := cluster.NewProvider(
		cluster.ProviderWithLogger(logger),
	)

	if err := prov.Delete(i.RuntimeInstanceName, i.KubeconfigPath); err != nil {
		return fmt.Errorf("failed to delete kind cluster: %w", err)
	}

	return nil
}

// getKindConfig returns a kind config for a threeport Kubernetes runtime.
func getKindConfig(devEnvironment bool, threeportPath string, numWorkerNodes int) *v1alpha4.Cluster {
	clusterConfig := v1alpha4.Cluster{}

	var controlPlaneNode v1alpha4.Node
	var workerNodes []v1alpha4.Node
	if devEnvironment {
		controlPlaneNode = *devEnvKindControlPlaneNode(threeportPath)
		workerNodes = *devEnvKindWorkers(threeportPath, numWorkerNodes)
	} else {
		controlPlaneNode = *kindControlPlaneNode()
		workerNodes = *kindWorkers(numWorkerNodes)
	}
	clusterConfig.Nodes = []v1alpha4.Node{controlPlaneNode}
	for _, n := range workerNodes {
		clusterConfig.Nodes = append(clusterConfig.Nodes, n)
	}

	return &clusterConfig
}

// devEnvKindControlPlaneNode returns a control plane node with host path mount
// for live code reloads.
func devEnvKindControlPlaneNode(threeportPath string) *v1alpha4.Node {
	controlPlaneNode := v1alpha4.Node{
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
				ContainerPort: int32(30000),
				HostPort:      int32(443),
				Protocol:      v1alpha4.PortMappingProtocolTCP,
			},
		},
		ExtraMounts: []v1alpha4.Mount{
			{
				ContainerPath: "/threeport",
				HostPath:      threeportPath,
			},
		},
	}

	return &controlPlaneNode
}

// kindControlPlaneNode returns a control plane node config for regular use.
func kindControlPlaneNode() *v1alpha4.Node {
	controlPlaneNode := v1alpha4.Node{
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
				ContainerPort: int32(30000),
				HostPort:      int32(443),
				Protocol:      v1alpha4.PortMappingProtocolTCP,
			},
		},
	}

	return &controlPlaneNode
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

// kindWorkers returns regular worker nodes.
func kindWorkers(numWorkerNodes int) *[]v1alpha4.Node {
	nodes := make([]v1alpha4.Node, numWorkerNodes)
	for i := range nodes {

		nodes[i] = v1alpha4.Node{
			Role: v1alpha4.WorkerRole,
		}

	}

	return &nodes
}
