package provider

import (
	"fmt"
	"os"
	"time"

	"sigs.k8s.io/kind/pkg/apis/config/v1alpha4"
	"sigs.k8s.io/kind/pkg/cluster"
	"sigs.k8s.io/kind/pkg/cmd"

	"github.com/threeport/threeport/internal/kube"
)

// KubernetesRuntimeInfraKind represents a kind cluster for local a threeport instance.
type KubernetesRuntimeInfraKind struct {
	// The unique name of the kubernetes runtime instance.
	RuntimeInstanceName string

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

	// True if Threeport API is served via HTTPs.
	AuthEnabled bool
}

// Create installs a Kubernetes cluster using kind for the threeport control
// plane.
func (i *KubernetesRuntimeInfraKind) Create() (*kube.KubeConnectionInfo, error) {
	logger := cmd.NewLogger()
	prov := cluster.NewProvider(
		cluster.ProviderWithLogger(logger),
	)

	// create the kind cluster
	if err := prov.Create(
		i.RuntimeInstanceName,
		cluster.CreateWithKubeconfigPath(i.KubeconfigPath),
		cluster.CreateWithWaitForReady(time.Duration(1000000000*60*5)), // 5 minutes
		cluster.CreateWithV1Alpha4Config(getKindConfig(i.AuthEnabled, i.DevEnvironment, i.ThreeportPath, i.NumWorkerNodes)),
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
func (i *KubernetesRuntimeInfraKind) Delete() error {
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
func getKindConfig(authEnabled, devEnvironment bool, threeportPath string, numWorkerNodes int) *v1alpha4.Cluster {
	clusterConfig := v1alpha4.Cluster{}

	var controlPlaneNode v1alpha4.Node
	var workerNodes []v1alpha4.Node
	if devEnvironment {

		// configure goPath, default to home directory if not set
		var goPath string
		goPath = os.Getenv("GOPATH")
		if goPath == "" {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				panic(err)
			}
			goPath = homeDir + "/go"
		}

		// configure goCache, default to ~/.cache/go-build if not set
		var goCache string
		goCache = os.Getenv("GOCACHE")
		if goCache == "" {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				panic(err)
			}
			goCache = homeDir + "/.cache/go-build"
		}

		controlPlaneNode = *devEnvKindControlPlaneNode(authEnabled, threeportPath, goPath, goCache)
		workerNodes = *devEnvKindWorkers(threeportPath, numWorkerNodes, goPath, goCache)
	} else {
		controlPlaneNode = *kindControlPlaneNode(authEnabled)
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
func devEnvKindControlPlaneNode(authEnabled bool, threeportPath, goPath, goCache string) *v1alpha4.Node {
	hostPort := kube.GetThreeportAPIPort(authEnabled)
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
				HostPort:      int32(hostPort),
				Protocol:      v1alpha4.PortMappingProtocolTCP,
			},
		},
		ExtraMounts: []v1alpha4.Mount{
			{
				ContainerPath: "/threeport",
				HostPath:      threeportPath,
			},
			{
				ContainerPath: "/go",
				HostPath:      goPath,
			},
			{
				ContainerPath: "/root/.cache/go-build",
				HostPath:      goCache,
			},
		},
	}

	return &controlPlaneNode
}

// kindControlPlaneNode returns a control plane node config for regular use.
func kindControlPlaneNode(authEnabled bool) *v1alpha4.Node {
	hostPort := kube.GetThreeportAPIPort(authEnabled)
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
				HostPort:      int32(hostPort),
				Protocol:      v1alpha4.PortMappingProtocolTCP,
			},
		},
	}

	return &controlPlaneNode
}

// devEnvKindWorkers returns worker nodes with host path mount for live code
// reloads.
func devEnvKindWorkers(threeportPath string, numWorkerNodes int, goPath, goCache string) *[]v1alpha4.Node {

	nodes := make([]v1alpha4.Node, numWorkerNodes)
	for i := range nodes {
		nodes[i] = v1alpha4.Node{
			Role: v1alpha4.WorkerRole,
			ExtraMounts: []v1alpha4.Mount{
				{
					ContainerPath: "/threeport",
					HostPath:      threeportPath,
				},
				{
					ContainerPath: "/go",
					HostPath:      goPath,
				},
				{
					ContainerPath: "/root/.cache/go-build",
					HostPath:      goCache,
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
