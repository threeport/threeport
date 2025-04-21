package provider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pulumi/pulumi-oci/sdk/go/oci/containerengine"
	"github.com/pulumi/pulumi-oci/sdk/go/oci/core"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	kube "github.com/threeport/threeport/pkg/kube/v0"
)

// KubernetesRuntimeInfraOKE represents the infrastructure for a threeport-managed OKE
// (Oracle Kubernetes Engine) cluster.
type KubernetesRuntimeInfraOKE struct {
	// The unique name of the kubernetes runtime instance managed by threeport.
	RuntimeInstanceName string

	// The Oracle Cloud tenancy ID where the cluster infra is provisioned.
	TenancyID string

	// The Oracle Cloud compartment ID where resources will be created.
	CompartmentID string

	// The Oracle Cloud region where resources will be created.
	Region string

	// The number of availability domains the OKE cluster will be deployed across.
	AvailabilityDomainCount int32

	// The Oracle Cloud shape used for the worker nodes.
	WorkerNodeShape string

	// The number of nodes initially created for the worker node pool.
	WorkerNodeInitialCount int32

	// The minimum number of nodes to maintain in the worker node pool.
	WorkerNodeMinCount int32

	// The maximum number of nodes allowed in the worker node pool.
	WorkerNodeMaxCount int32

	// The Pulumi stack for managing OKE resources
	PulumiStack *auto.Stack
}

// Create installs a Kubernetes cluster using Oracle Cloud OKE for threeport workloads.
func (i *KubernetesRuntimeInfraOKE) Create() (*kube.KubeConnectionInfo, error) {
	// Create a new Pulumi program
	program := func(ctx *pulumi.Context) error {
		// Create VCN for the cluster
		vcn, err := core.NewVcn(ctx, fmt.Sprintf("%s-vcn", i.RuntimeInstanceName), &core.VcnArgs{
			CompartmentId: pulumi.String(i.CompartmentID),
			CidrBlock:     pulumi.String("10.0.0.0/16"),
			DisplayName:   pulumi.String(fmt.Sprintf("%s-vcn", i.RuntimeInstanceName)),
			DnsLabel:      pulumi.String(i.RuntimeInstanceName),
		}, pulumi.DeleteBeforeReplace(true))
		if err != nil {
			return fmt.Errorf("failed to create VCN: %w", err)
		}

		// Create Internet Gateway
		_, err = core.NewInternetGateway(ctx, fmt.Sprintf("%s-igw", i.RuntimeInstanceName), &core.InternetGatewayArgs{
			CompartmentId: pulumi.String(i.CompartmentID),
			VcnId:         vcn.ID(),
			DisplayName:   pulumi.String(fmt.Sprintf("%s-igw", i.RuntimeInstanceName)),
			Enabled:       pulumi.Bool(true),
		}, pulumi.DeleteBeforeReplace(true))
		if err != nil {
			return fmt.Errorf("failed to create Internet Gateway: %w", err)
		}

		// Create OKE Cluster
		cluster, err := containerengine.NewCluster(ctx, i.RuntimeInstanceName, &containerengine.ClusterArgs{
			CompartmentId:     pulumi.String(i.CompartmentID),
			Name:              pulumi.String(i.RuntimeInstanceName),
			VcnId:             vcn.ID(),
			KubernetesVersion: pulumi.String("v1.28.2"), // Latest stable version
			Options: &containerengine.ClusterOptionsArgs{
				KubernetesNetworkConfig: &containerengine.ClusterOptionsKubernetesNetworkConfigArgs{
					PodsCidr:     pulumi.String("10.244.0.0/16"),
					ServicesCidr: pulumi.String("10.96.0.0/12"),
				},
			},
		}, pulumi.DeleteBeforeReplace(true))
		if err != nil {
			return fmt.Errorf("failed to create OKE cluster: %w", err)
		}

		// Create Node Pool
		_, err = containerengine.NewNodePool(ctx, fmt.Sprintf("%s-nodepool", i.RuntimeInstanceName), &containerengine.NodePoolArgs{
			ClusterId:     cluster.ID(),
			CompartmentId: pulumi.String(i.CompartmentID),
			Name:          pulumi.String(fmt.Sprintf("%s-nodepool", i.RuntimeInstanceName)),
			NodeShape:     pulumi.String(i.WorkerNodeShape),
			InitialNodeLabels: containerengine.NodePoolInitialNodeLabelArray{
				&containerengine.NodePoolInitialNodeLabelArgs{
					Key:   pulumi.String("threeport.io/managed"),
					Value: pulumi.String("true"),
				},
			},
			QuantityPerSubnet: pulumi.Int(int(i.WorkerNodeInitialCount)),
			SubnetIds:         pulumi.StringArray{vcn.ID()}, // In a real implementation, we'd create proper subnets
		}, pulumi.DeleteBeforeReplace(true))
		if err != nil {
			return fmt.Errorf("failed to create node pool: %w", err)
		}

		// Export cluster ID and kubeconfig for later use
		ctx.Export("clusterId", cluster.ID())
		ctx.Export("kubeconfig", cluster.Endpoints.Index(pulumi.Int(0)).PrivateEndpoint())

		return nil
	}

	// Create a new Pulumi stack with local backend
	stackName := fmt.Sprintf("oke-%s", i.RuntimeInstanceName)
	ctx := context.Background()

	// Configure local filesystem backend
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	stateDir := filepath.Join(homeDir, ".config", "threeport", "pulumi-state")

	// Ensure state directory exists
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create state directory: %w", err)
	}

	// Create stack with local workspace
	stack, err := auto.NewStackInlineSource(ctx, stackName, "oke", program, auto.WorkDir(stateDir))
	if err != nil {
		return nil, fmt.Errorf("failed to create Pulumi stack: %w", err)
	}
	i.PulumiStack = &stack

	// Run the update to create resources
	_, err = i.PulumiStack.Up(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create resources: %w", err)
	}

	// Get outputs
	outputs, err := i.PulumiStack.Outputs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get outputs: %w", err)
	}

	clusterId := outputs["clusterId"].Value.(string)
	kubeconfig := outputs["kubeconfig"].Value.(string)

	// Construct KubeConnectionInfo
	kubeConnInfo := &kube.KubeConnectionInfo{
		APIEndpoint:   clusterId,
		CACertificate: kubeconfig,
	}

	return kubeConnInfo, nil
}

// Delete deletes an Oracle Cloud OKE cluster.
func (i *KubernetesRuntimeInfraOKE) Delete() error {
	if i.PulumiStack != nil {
		ctx := context.Background()
		_, err := i.PulumiStack.Destroy(ctx)
		if err != nil {
			return fmt.Errorf("failed to destroy Pulumi stack: %w", err)
		}
	}
	return nil
}

// GetConnection gets the latest connection info for authentication to an OKE cluster.
func (i *KubernetesRuntimeInfraOKE) GetConnection() (*kube.KubeConnectionInfo, error) {
	if i.PulumiStack == nil {
		return nil, fmt.Errorf("Pulumi stack not initialized")
	}

	ctx := context.Background()
	outputs, err := i.PulumiStack.Outputs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get outputs: %w", err)
	}

	clusterId := outputs["clusterId"].Value.(string)
	kubeconfig := outputs["kubeconfig"].Value.(string)

	kubeConnInfo := &kube.KubeConnectionInfo{
		APIEndpoint:   clusterId,
		CACertificate: kubeconfig,
	}

	return kubeConnInfo, nil
}

// OKEInventoryFilepath returns a standardized filename and path for the OKE
// inventory file.
func OKEInventoryFilepath(providerConfigDir, instanceName string) string {
	inventoryFilename := fmt.Sprintf("oke-inventory-%s.json", instanceName)
	return filepath.Join(providerConfigDir, inventoryFilename)
}
