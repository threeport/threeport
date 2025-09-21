package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/common"
	ocicontainerengine "github.com/oracle/oci-go-sdk/v65/containerengine"
	ocicore "github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"
	"github.com/pulumi/pulumi-oci/sdk/v2/go/oci"
	"github.com/pulumi/pulumi-oci/sdk/v2/go/oci/containerengine"
	"github.com/pulumi/pulumi-oci/sdk/v2/go/oci/core"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optdestroy"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
	"gopkg.in/ini.v1"
	"gopkg.in/yaml.v2"
	"gorm.io/datatypes"
)

// KubernetesRuntimeInfraOKE represents the infrastructure for a threeport-managed OKE
// (Oracle Kubernetes Engine) cluster.
type KubernetesRuntimeInfraOKE struct {
	// The unique name of the kubernetes runtime instance managed by threeport.
	RuntimeInstanceName string

	// Version of the OKE cluster.
	Version string

	// The Oracle Cloud tenancy ID where the cluster infra is provisioned.
	TenancyOCID string

	// The Oracle Cloud compartment ID where resources will be created.
	CompartmentOCID string

	// The Oracle Cloud region where resources will be created.
	Region string

	// The Oracle Cloud shape used for the worker nodes.
	WorkerNodeShape string

	// The number of nodes initially created for the worker node pool.
	WorkerNodeInitialCount int32

	// The path to the Pulumi state directory
	stateDir string
}

// Create installs a Kubernetes cluster using Oracle Cloud OKE for threeport workloads.
func (i *KubernetesRuntimeInfraOKE) Create() (*kube.KubeConnectionInfo, error) {
	// set up Pulumi workspace and get stack
	stack, err := i.setupPulumiWorkspace(func(ctx *pulumi.Context) error {

		// get the availability domain name
		availabilityDomain, err := i.getAvailabilityDomainName()
		if err != nil {
			return fmt.Errorf("failed to get availability domain: %w", err)
		}

		// create OCI provider with explicit configuration
		ociProvider, err := oci.NewProvider(ctx, "oci-provider", &oci.ProviderArgs{
			Region:      pulumi.String(i.Region),
			TenancyOcid: pulumi.String(i.TenancyOCID),
		})
		if err != nil {
			return fmt.Errorf("failed to create OCI provider: %w", err)
		}

		// create VCN for the cluster
		vcn, err := core.NewVcn(ctx, fmt.Sprintf("%s-vcn", i.RuntimeInstanceName), &core.VcnArgs{
			CompartmentId: pulumi.String(i.CompartmentOCID),
			CidrBlock:     pulumi.String("10.0.0.0/16"),
			DisplayName:   pulumi.String(fmt.Sprintf("%s-vcn", i.RuntimeInstanceName)),
			DnsLabel:      pulumi.String(createDNSLabel(i.RuntimeInstanceName)),
			IsIpv6enabled: pulumi.Bool(false),
		}, pulumi.Provider(ociProvider),
			pulumi.DeleteBeforeReplace(true),
			pulumi.Protect(false))
		if err != nil {
			return fmt.Errorf("failed to create VCN: %w", err)
		}

		// create Internet Gateway
		internetGateway, err := core.NewInternetGateway(ctx, fmt.Sprintf("%s-ig", i.RuntimeInstanceName), &core.InternetGatewayArgs{
			CompartmentId: pulumi.String(i.CompartmentOCID),
			VcnId:         vcn.ID(),
			DisplayName:   pulumi.String(fmt.Sprintf("%s-ig", i.RuntimeInstanceName)),
			Enabled:       pulumi.Bool(true),
		}, pulumi.Provider(ociProvider),
			pulumi.DependsOn([]pulumi.Resource{vcn}))
		if err != nil {
			return fmt.Errorf("failed to create Internet Gateway: %w", err)
		}

		// create NAT Gateway
		natGateway, err := core.NewNatGateway(ctx, fmt.Sprintf("%s-ng", i.RuntimeInstanceName), &core.NatGatewayArgs{
			CompartmentId: pulumi.String(i.CompartmentOCID),
			VcnId:         vcn.ID(),
			DisplayName:   pulumi.String(fmt.Sprintf("%s-ng", i.RuntimeInstanceName)),
			BlockTraffic:  pulumi.Bool(false),
		}, pulumi.Provider(ociProvider),
			pulumi.DependsOn([]pulumi.Resource{vcn}))
		if err != nil {
			return fmt.Errorf("failed to create NAT Gateway: %w", err)
		}

		// get the service gateway service ID
		serviceID, err := getServiceGatewayServiceID(i.Region, i.CompartmentOCID)
		if err != nil {
			return fmt.Errorf("failed to get service gateway service ID: %w", err)
		}

		// create Service Gateway
		serviceGateway, err := core.NewServiceGateway(ctx, fmt.Sprintf("%s-sg", i.RuntimeInstanceName), &core.ServiceGatewayArgs{
			CompartmentId: pulumi.String(i.CompartmentOCID),
			VcnId:         vcn.ID(),
			DisplayName:   pulumi.String(fmt.Sprintf("%s-sg", i.RuntimeInstanceName)),
			Services: core.ServiceGatewayServiceArray{
				&core.ServiceGatewayServiceArgs{
					ServiceId: pulumi.String(serviceID),
				},
			},
		}, pulumi.Provider(ociProvider),
			pulumi.DependsOn([]pulumi.Resource{vcn}))
		if err != nil {
			return fmt.Errorf("failed to create Service Gateway: %w", err)
		}

		// create route table for public subnet
		publicRouteTable, err := core.NewRouteTable(ctx, fmt.Sprintf("%s-public-rt", i.RuntimeInstanceName), &core.RouteTableArgs{
			CompartmentId: pulumi.String(i.CompartmentOCID),
			VcnId:         vcn.ID(),
			DisplayName:   pulumi.String(fmt.Sprintf("%s-public-rt", i.RuntimeInstanceName)),
			RouteRules: core.RouteTableRouteRuleArray{
				&core.RouteTableRouteRuleArgs{
					Destination:     pulumi.String("0.0.0.0/0"),
					DestinationType: pulumi.String("CIDR_BLOCK"),
					NetworkEntityId: internetGateway.ID(),
				},
			},
		}, pulumi.Provider(ociProvider),
			pulumi.DependsOn([]pulumi.Resource{internetGateway}))
		if err != nil {
			return fmt.Errorf("failed to create public route table: %w", err)
		}

		// create route table for private subnet
		privateRouteTable, err := core.NewRouteTable(ctx, fmt.Sprintf("%s-private-rt", i.RuntimeInstanceName), &core.RouteTableArgs{
			CompartmentId: pulumi.String(i.CompartmentOCID),
			VcnId:         vcn.ID(),
			DisplayName:   pulumi.String(fmt.Sprintf("%s-private-rt", i.RuntimeInstanceName)),
			RouteRules: core.RouteTableRouteRuleArray{
				&core.RouteTableRouteRuleArgs{
					Destination:     pulumi.String("0.0.0.0/0"),
					DestinationType: pulumi.String("CIDR_BLOCK"),
					NetworkEntityId: natGateway.ID(),
				},
				&core.RouteTableRouteRuleArgs{
					Destination:     pulumi.String("all-phx-services-in-oracle-services-network"),
					DestinationType: pulumi.String("SERVICE_CIDR_BLOCK"),
					NetworkEntityId: serviceGateway.ID(),
				},
			},
		}, pulumi.Provider(ociProvider),
			pulumi.DependsOn([]pulumi.Resource{natGateway, serviceGateway}))
		if err != nil {
			return fmt.Errorf("failed to create private route table: %w", err)
		}

		// define subnets to be used by security lists, cluster, and load balancer
		publicSubnetCidrBlock := "10.0.0.0/28"
		privateSubnetCidrBlock := "10.0.10.0/24"
		loadBalancerSubnetCidrBlock := "10.0.20.0/24"

		// create security list for public subnet
		publicSecList, err := core.NewSecurityList(ctx, fmt.Sprintf("%s-public-seclist", i.RuntimeInstanceName), &core.SecurityListArgs{
			CompartmentId: pulumi.String(i.CompartmentOCID),
			VcnId:         vcn.ID(),
			DisplayName:   pulumi.String(fmt.Sprintf("%s-public-seclist", i.RuntimeInstanceName)),
			IngressSecurityRules: core.SecurityListIngressSecurityRuleArray{
				// allow Kubernetes API server traffic from anywhere
				&core.SecurityListIngressSecurityRuleArgs{
					Protocol: pulumi.String("6"), // TCP
					Source:   pulumi.String("0.0.0.0/0"),
					TcpOptions: &core.SecurityListIngressSecurityRuleTcpOptionsArgs{
						Max: pulumi.Int(6443),
						Min: pulumi.Int(6443),
					},
					Stateless: pulumi.Bool(false),
				},
				// allow Kubernetes API server traffic from private subnet
				&core.SecurityListIngressSecurityRuleArgs{
					Protocol: pulumi.String("6"), // TCP
					Source:   pulumi.String(privateSubnetCidrBlock),
					TcpOptions: &core.SecurityListIngressSecurityRuleTcpOptionsArgs{
						Max: pulumi.Int(6443),
						Min: pulumi.Int(6443),
					},
					Stateless: pulumi.Bool(false),
				},
				// allow port 12250 from private subnet
				&core.SecurityListIngressSecurityRuleArgs{
					Protocol: pulumi.String("6"), // TCP
					Source:   pulumi.String(privateSubnetCidrBlock),
					TcpOptions: &core.SecurityListIngressSecurityRuleTcpOptionsArgs{
						Max: pulumi.Int(12250),
						Min: pulumi.Int(12250),
					},
					Stateless: pulumi.Bool(false),
				},
				// allow ICMP type 3 code 4 from private subnet
				&core.SecurityListIngressSecurityRuleArgs{
					Protocol: pulumi.String("1"), // ICMP
					Source:   pulumi.String(privateSubnetCidrBlock),
					IcmpOptions: &core.SecurityListIngressSecurityRuleIcmpOptionsArgs{
						Type: pulumi.Int(3),
						Code: pulumi.Int(4),
					},
					Stateless: pulumi.Bool(false),
				},
			},
			EgressSecurityRules: core.SecurityListEgressSecurityRuleArray{
				// allow traffic to Oracle Services Network
				&core.SecurityListEgressSecurityRuleArgs{
					Protocol:        pulumi.String("6"), // TCP
					Destination:     pulumi.String("all-phx-services-in-oracle-services-network"),
					DestinationType: pulumi.String("SERVICE_CIDR_BLOCK"),
					TcpOptions: &core.SecurityListEgressSecurityRuleTcpOptionsArgs{
						Max: pulumi.Int(443),
						Min: pulumi.Int(443),
					},
					Stateless: pulumi.Bool(false),
				},
				// allow all TCP traffic to private subnet
				&core.SecurityListEgressSecurityRuleArgs{
					Protocol:    pulumi.String("6"), // TCP
					Destination: pulumi.String(privateSubnetCidrBlock),
					Stateless:   pulumi.Bool(false),
				},
				// allow ICMP type 3 code 4 to private subnet
				&core.SecurityListEgressSecurityRuleArgs{
					Protocol:    pulumi.String("1"), // ICMP
					Destination: pulumi.String(privateSubnetCidrBlock),
					IcmpOptions: &core.SecurityListEgressSecurityRuleIcmpOptionsArgs{
						Type: pulumi.Int(3),
						Code: pulumi.Int(4),
					},
					Stateless: pulumi.Bool(false),
				},
			},
		}, pulumi.Provider(ociProvider),
			pulumi.DependsOn([]pulumi.Resource{vcn}))
		if err != nil {
			return fmt.Errorf("failed to create public security list: %w", err)
		}

		// create security list for worker nodes subnet (private)
		workerNodesSecList, err := core.NewSecurityList(ctx, fmt.Sprintf("%s-worker-nodes-seclist", i.RuntimeInstanceName), &core.SecurityListArgs{
			CompartmentId: pulumi.String(i.CompartmentOCID),
			VcnId:         vcn.ID(),
			DisplayName:   pulumi.String(fmt.Sprintf("%s-worker-nodes-seclist", i.RuntimeInstanceName)),
			IngressSecurityRules: core.SecurityListIngressSecurityRuleArray{
				// allow all traffic from private subnet
				&core.SecurityListIngressSecurityRuleArgs{
					Protocol:  pulumi.String("all"),
					Source:    pulumi.String(privateSubnetCidrBlock),
					Stateless: pulumi.Bool(false),
				},
				// allow ICMP type 3 code 4 from public subnet
				&core.SecurityListIngressSecurityRuleArgs{
					Protocol: pulumi.String("1"), // ICMP
					Source:   pulumi.String(publicSubnetCidrBlock),
					IcmpOptions: &core.SecurityListIngressSecurityRuleIcmpOptionsArgs{
						Type: pulumi.Int(3),
						Code: pulumi.Int(4),
					},
					Stateless: pulumi.Bool(false),
				},
				// allow all TCP traffic from public subnet
				&core.SecurityListIngressSecurityRuleArgs{
					Protocol:  pulumi.String("6"), // TCP
					Source:    pulumi.String(publicSubnetCidrBlock),
					Stateless: pulumi.Bool(false),
				},
				// allow SSH from anywhere
				&core.SecurityListIngressSecurityRuleArgs{
					Protocol: pulumi.String("6"), // TCP
					Source:   pulumi.String("0.0.0.0/0"),
					TcpOptions: &core.SecurityListIngressSecurityRuleTcpOptionsArgs{
						Max: pulumi.Int(22),
						Min: pulumi.Int(22),
					},
					Stateless: pulumi.Bool(false),
				},
				// allow all traffic from load balancer subnet
				&core.SecurityListIngressSecurityRuleArgs{
					Protocol:  pulumi.String("all"),
					Source:    pulumi.String(loadBalancerSubnetCidrBlock),
					Stateless: pulumi.Bool(false),
				},
			},
			EgressSecurityRules: core.SecurityListEgressSecurityRuleArray{
				// allow all traffic to private subnet
				&core.SecurityListEgressSecurityRuleArgs{
					Protocol:    pulumi.String("all"),
					Destination: pulumi.String(privateSubnetCidrBlock),
					Stateless:   pulumi.Bool(false),
				},
				// allow Kubernetes API server traffic to public subnet
				&core.SecurityListEgressSecurityRuleArgs{
					Protocol:    pulumi.String("6"), // TCP
					Destination: pulumi.String(publicSubnetCidrBlock),
					TcpOptions: &core.SecurityListEgressSecurityRuleTcpOptionsArgs{
						Max: pulumi.Int(6443),
						Min: pulumi.Int(6443),
					},
					Stateless: pulumi.Bool(false),
				},
				// allow TCP port 12250 to public subnet
				&core.SecurityListEgressSecurityRuleArgs{
					Protocol:    pulumi.String("6"), // TCP
					Destination: pulumi.String(publicSubnetCidrBlock),
					TcpOptions: &core.SecurityListEgressSecurityRuleTcpOptionsArgs{
						Max: pulumi.Int(12250),
						Min: pulumi.Int(12250),
					},
					Stateless: pulumi.Bool(false),
				},
				// allow ICMP type 3 code 4 to public subnet
				&core.SecurityListEgressSecurityRuleArgs{
					Protocol:    pulumi.String("1"), // ICMP
					Destination: pulumi.String(publicSubnetCidrBlock),
					IcmpOptions: &core.SecurityListEgressSecurityRuleIcmpOptionsArgs{
						Type: pulumi.Int(3),
						Code: pulumi.Int(4),
					},
					Stateless: pulumi.Bool(false),
				},
				// allow TCP port 443 to Oracle Services Network
				&core.SecurityListEgressSecurityRuleArgs{
					Protocol:        pulumi.String("6"), // TCP
					Destination:     pulumi.String("all-phx-services-in-oracle-services-network"),
					DestinationType: pulumi.String("SERVICE_CIDR_BLOCK"),
					TcpOptions: &core.SecurityListEgressSecurityRuleTcpOptionsArgs{
						Max: pulumi.Int(443),
						Min: pulumi.Int(443),
					},
					Stateless: pulumi.Bool(false),
				},
				// allow ICMP type 3 code 4 to anywhere
				&core.SecurityListEgressSecurityRuleArgs{
					Protocol:    pulumi.String("1"), // ICMP
					Destination: pulumi.String("0.0.0.0/0"),
					IcmpOptions: &core.SecurityListEgressSecurityRuleIcmpOptionsArgs{
						Type: pulumi.Int(3),
						Code: pulumi.Int(4),
					},
					Stateless: pulumi.Bool(false),
				},
				// allow all traffic to anywhere
				&core.SecurityListEgressSecurityRuleArgs{
					Protocol:    pulumi.String("all"),
					Destination: pulumi.String("0.0.0.0/0"),
					Stateless:   pulumi.Bool(false),
				},
			},
		}, pulumi.Provider(ociProvider),
			pulumi.DependsOn([]pulumi.Resource{vcn}))
		if err != nil {
			return fmt.Errorf("failed to create worker nodes security list: %w", err)
		}

		// create load balancer security list
		loadBalancerSecList, err := core.NewSecurityList(ctx, fmt.Sprintf("%s-load-balancer-seclist", i.RuntimeInstanceName), &core.SecurityListArgs{
			CompartmentId: pulumi.String(i.CompartmentOCID),
			VcnId:         vcn.ID(),
			DisplayName:   pulumi.String(fmt.Sprintf("%s-load-balancer-seclist", i.RuntimeInstanceName)),
			IngressSecurityRules: core.SecurityListIngressSecurityRuleArray{
				// allow 443 from anywhere
				&core.SecurityListIngressSecurityRuleArgs{
					Protocol:  pulumi.String("6"), // TCP
					Source:    pulumi.String("0.0.0.0/0"),
					Stateless: pulumi.Bool(false),
					TcpOptions: &core.SecurityListIngressSecurityRuleTcpOptionsArgs{
						Max: pulumi.Int(443),
						Min: pulumi.Int(443),
					},
				},
				// allow 80 from anywhere
				&core.SecurityListIngressSecurityRuleArgs{
					Protocol:  pulumi.String("6"), // TCP
					Source:    pulumi.String("0.0.0.0/0"),
					Stateless: pulumi.Bool(false),
					TcpOptions: &core.SecurityListIngressSecurityRuleTcpOptionsArgs{
						Max: pulumi.Int(80),
						Min: pulumi.Int(80),
					},
				},
			},
			EgressSecurityRules: core.SecurityListEgressSecurityRuleArray{
				// allow all traffic to private subnet
				&core.SecurityListEgressSecurityRuleArgs{
					Protocol:    pulumi.String("all"),
					Destination: pulumi.String(privateSubnetCidrBlock),
					Stateless:   pulumi.Bool(false),
				},
			},
		}, pulumi.Provider(ociProvider),
			pulumi.DependsOn([]pulumi.Resource{vcn}))
		if err != nil {
			return fmt.Errorf("failed to create load balancer security list: %w", err)
		}

		// create public subnet
		publicSubnet, err := core.NewSubnet(ctx, fmt.Sprintf("%s-public-subnet", i.RuntimeInstanceName), &core.SubnetArgs{
			CidrBlock:              pulumi.String(publicSubnetCidrBlock),
			CompartmentId:          pulumi.String(i.CompartmentOCID),
			VcnId:                  vcn.ID(),
			DisplayName:            pulumi.String(fmt.Sprintf("%s-public-subnet", i.RuntimeInstanceName)),
			DnsLabel:               pulumi.String(createDNSLabel(fmt.Sprintf("%s-public", i.RuntimeInstanceName))),
			ProhibitPublicIpOnVnic: pulumi.Bool(false),
			RouteTableId:           publicRouteTable.ID(),
			SecurityListIds:        pulumi.StringArray{publicSecList.ID()},
		}, pulumi.Provider(ociProvider),
			pulumi.DependsOn([]pulumi.Resource{vcn, publicRouteTable, publicSecList}))
		if err != nil {
			return fmt.Errorf("failed to create public subnet: %w", err)
		}

		// create private subnet
		privateSubnet, err := core.NewSubnet(ctx, fmt.Sprintf("%s-private-subnet", i.RuntimeInstanceName), &core.SubnetArgs{
			CidrBlock:              pulumi.String(privateSubnetCidrBlock),
			CompartmentId:          pulumi.String(i.CompartmentOCID),
			VcnId:                  vcn.ID(),
			DisplayName:            pulumi.String(fmt.Sprintf("%s-private-subnet", i.RuntimeInstanceName)),
			DnsLabel:               pulumi.String(createDNSLabel(fmt.Sprintf("%s-private", i.RuntimeInstanceName))),
			ProhibitPublicIpOnVnic: pulumi.Bool(true),
			RouteTableId:           privateRouteTable.ID(),
			SecurityListIds:        pulumi.StringArray{workerNodesSecList.ID()},
		}, pulumi.Provider(ociProvider),
			pulumi.DependsOn([]pulumi.Resource{vcn, privateRouteTable, workerNodesSecList}))
		if err != nil {
			return fmt.Errorf("failed to create private subnet: %w", err)
		}

		// create load balancer subnet
		loadBalancerSubnet, err := core.NewSubnet(ctx, fmt.Sprintf("%s-lb-subnet", i.RuntimeInstanceName), &core.SubnetArgs{
			CidrBlock:              pulumi.String(loadBalancerSubnetCidrBlock),
			CompartmentId:          pulumi.String(i.CompartmentOCID),
			VcnId:                  vcn.ID(),
			DisplayName:            pulumi.String(fmt.Sprintf("%s-lb-subnet", i.RuntimeInstanceName)),
			DnsLabel:               pulumi.String(createDNSLabel(fmt.Sprintf("%s-lb", i.RuntimeInstanceName))),
			ProhibitPublicIpOnVnic: pulumi.Bool(false),
			RouteTableId:           publicRouteTable.ID(),
			SecurityListIds:        pulumi.StringArray{loadBalancerSecList.ID()},
		}, pulumi.Provider(ociProvider),
			pulumi.DependsOn([]pulumi.Resource{vcn, publicRouteTable, loadBalancerSecList}))
		if err != nil {
			return fmt.Errorf("failed to create load balancer subnet: %w", err)
		}

		// create OKE Cluster with explicit dependency on networking components
		cluster, err := containerengine.NewCluster(ctx, i.RuntimeInstanceName, &containerengine.ClusterArgs{
			CompartmentId:     pulumi.String(i.CompartmentOCID),
			Name:              pulumi.String(i.RuntimeInstanceName),
			VcnId:             vcn.ID(),
			KubernetesVersion: pulumi.String(i.Version),
			EndpointConfig: &containerengine.ClusterEndpointConfigArgs{
				IsPublicIpEnabled: pulumi.Bool(true),
				SubnetId:          publicSubnet.ID(),
				NsgIds:            pulumi.StringArray{}, // optional: Add network security group IDs if needed
			},
			Options: &containerengine.ClusterOptionsArgs{
				KubernetesNetworkConfig: &containerengine.ClusterOptionsKubernetesNetworkConfigArgs{
					PodsCidr:     pulumi.String("10.244.0.0/16"),
					ServicesCidr: pulumi.String("10.96.0.0/16"),
				},
				ServiceLbSubnetIds: pulumi.StringArray{loadBalancerSubnet.ID()},
			},
		}, pulumi.Provider(ociProvider),
			pulumi.DependsOn([]pulumi.Resource{
				vcn,
				internetGateway,
				natGateway,
				serviceGateway,
				publicSubnet,
				privateSubnet,
				publicRouteTable,
				privateRouteTable,
			}))
		if err != nil {
			return fmt.Errorf("failed to create OKE cluster: %w", err)
		}

		// get the OKE worker node image OCID
		imageOCID, err := i.getOKEWorkerNodeImageOCID()
		if err != nil {
			return fmt.Errorf("failed to get OKE worker node image OCID: %w", err)
		}

		// create node pool
		_, err = containerengine.NewNodePool(ctx, fmt.Sprintf("%s-nodepool", i.RuntimeInstanceName), &containerengine.NodePoolArgs{
			ClusterId:         cluster.ID(),
			CompartmentId:     pulumi.String(i.CompartmentOCID),
			Name:              pulumi.String(fmt.Sprintf("%s-nodepool", i.RuntimeInstanceName)),
			NodeShape:         pulumi.String(i.WorkerNodeShape),
			KubernetesVersion: pulumi.String(i.Version),
			InitialNodeLabels: containerengine.NodePoolInitialNodeLabelArray{
				&containerengine.NodePoolInitialNodeLabelArgs{
					Key:   pulumi.String("threeport.io/managed"),
					Value: pulumi.String("true"),
				},
			},
			NodeConfigDetails: &containerengine.NodePoolNodeConfigDetailsArgs{
				Size: pulumi.Int(int(i.WorkerNodeInitialCount)),
				PlacementConfigs: containerengine.NodePoolNodeConfigDetailsPlacementConfigArray{
					&containerengine.NodePoolNodeConfigDetailsPlacementConfigArgs{
						AvailabilityDomain: pulumi.String(availabilityDomain),
						SubnetId:           privateSubnet.ID(),
					},
				},
			},
			NodeSourceDetails: &containerengine.NodePoolNodeSourceDetailsArgs{
				ImageId:    pulumi.String(imageOCID),
				SourceType: pulumi.String("IMAGE"),
			},
			NodeShapeConfig: &containerengine.NodePoolNodeShapeConfigArgs{
				Ocpus:       pulumi.Float64(2.0),
				MemoryInGbs: pulumi.Float64(12.0),
			},
		}, pulumi.Provider(ociProvider),
			pulumi.DependsOn([]pulumi.Resource{cluster}))
		if err != nil {
			fmt.Printf("Failed to create node pool: %v\n", err)
			return fmt.Errorf("failed to create node pool: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to set up Pulumi workspace: %w", err)
	}

	// create a context for the automation API
	ctx := context.Background()

	// deploy the stack
	_, err = stack.Up(ctx, optup.ProgressStreams(os.Stdout))
	if err != nil {
		return nil, fmt.Errorf("failed to deploy stack: %w", err)
	}

	return i.GetConnection()
}

// Delete deletes an Oracle Cloud OKE cluster.
func (i *KubernetesRuntimeInfraOKE) Delete() error {
	// set up Pulumi workspace and get stack
	stack, err := i.setupPulumiWorkspace(func(ctx *pulumi.Context) error {
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to set up Pulumi workspace: %w", err)
	}

	// create a context for the automation API
	ctx := context.Background()

	// destroy the stack
	_, err = stack.Destroy(ctx, optdestroy.ProgressStreams(os.Stdout))
	if err != nil {
		return fmt.Errorf("failed to destroy stack: %w", err)
	}

	// remove the state directory after successful destruction
	if err := os.RemoveAll(i.stateDir); err != nil {
		return fmt.Errorf("failed to remove state directory: %w", err)
	}

	return nil
}

// GetClusterOCID gets the OCID of the OKE cluster.
func (i *KubernetesRuntimeInfraOKE) GetClusterOCID(
	okeClusterName string,
	configProvider common.ConfigurationProvider,
) (string, error) {

	containerClient, err := ocicontainerengine.NewContainerEngineClientWithConfigurationProvider(configProvider)
	if err != nil {
		return "", fmt.Errorf("failed to create container engine client: %w", err)
	}

	// set the region for the client
	containerClient.SetRegion(i.Region)

	// list clusters to find the one with matching name
	request := ocicontainerengine.ListClustersRequest{
		CompartmentId: &i.CompartmentOCID,
		Name:          &i.RuntimeInstanceName,
		LifecycleState: []ocicontainerengine.ClusterLifecycleStateEnum{
			ocicontainerengine.ClusterLifecycleStateActive,
		},
	}

	response, err := containerClient.ListClusters(context.Background(), request)
	if err != nil {
		return "", fmt.Errorf("failed to list clusters: %w", err)
	}

	// find the cluster with the matching name
	for _, cluster := range response.Items {
		if cluster.Name != nil && *cluster.Name == okeClusterName {
			return *cluster.Id, nil
		}
	}

	return "", fmt.Errorf("no active cluster found with name %s", okeClusterName)
}

// GetConnection gets the latest connection info for authentication to an OKE cluster.
func (i *KubernetesRuntimeInfraOKE) GetConnection() (*kube.KubeConnectionInfo, error) {
	// load OCI configuration first
	if err := i.loadOCIConfig(); err != nil {
		return nil, fmt.Errorf("failed to load OCI configuration: %w", err)
	}

	// create a new container engine client
	configProvider := common.DefaultConfigProvider()
	clusterOCID, err := i.GetClusterOCID(i.RuntimeInstanceName, configProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster OCID: %w", err)
	}

	containerClient, err := ocicontainerengine.NewContainerEngineClientWithConfigurationProvider(configProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to create container engine client: %w", err)
	}

	// get cluster details to get the API endpoint
	getClusterRequest := ocicontainerengine.GetClusterRequest{
		ClusterId: &clusterOCID,
	}

	clusterDetails, err := containerClient.GetCluster(context.Background(), getClusterRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster details: %w", err)
	}

	if clusterDetails.Endpoints == nil || clusterDetails.Endpoints.PublicEndpoint == nil {
		return nil, fmt.Errorf("cluster endpoints not found")
	}

	// get the kubeconfig which contains the CA certificate
	kubeconfigRequest := ocicontainerengine.CreateKubeconfigRequest{
		ClusterId: &clusterOCID,
		CreateClusterKubeconfigContentDetails: ocicontainerengine.CreateClusterKubeconfigContentDetails{
			TokenVersion: common.String("2.0.0"),
			Expiration:   common.Int(86400),
		},
	}

	kubeconfigResponse, err := containerClient.CreateKubeconfig(context.Background(), kubeconfigRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig: %w", err)
	}
	defer kubeconfigResponse.Content.Close()

	// read the kubeconfig content
	kubeconfigBytes, err := io.ReadAll(kubeconfigResponse.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to read kubeconfig content: %w", err)
	}

	// parse the kubeconfig using the KubeConfig struct
	var kubeconfig KubeConfig
	if err := yaml.Unmarshal(kubeconfigBytes, &kubeconfig); err != nil {
		return nil, fmt.Errorf("failed to parse kubeconfig: %w", err)
	}

	// validate and extract required fields
	if len(kubeconfig.Clusters) == 0 {
		return nil, fmt.Errorf("no clusters found in kubeconfig")
	}

	token, tokenExpirationTime, err := util.GenerateOkeToken(clusterOCID, configProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	caCert, err := base64.StdEncoding.DecodeString(kubeconfig.Clusters[0].Cluster.CertificateAuthorityData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode CA certificate: %w", err)
	}

	// create connection info
	kubeConnInfo := &kube.KubeConnectionInfo{
		APIEndpoint:     *clusterDetails.Endpoints.PublicEndpoint,
		CACertificate:   string(caCert),
		Token:           token,
		TokenExpiration: tokenExpirationTime,
	}

	return kubeConnInfo, nil
}

// KubeConfig represents the structure of the kubeconfig file
type KubeConfig struct {
	Clusters []struct {
		Cluster struct {
			CertificateAuthorityData string `yaml:"certificate-authority-data"`
		} `yaml:"cluster"`
	} `yaml:"clusters"`
}

// loadOCIConfig reads the OCI configuration using the OCI SDK and
// populates KubernetesRuntimeInfraOKE struct fields
func (i *KubernetesRuntimeInfraOKE) loadOCIConfig() error {
	// get user's home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	// path to OCI config file
	ociConfigPath := filepath.Join(homeDir, ".oci", "config")

	// check if config file exists
	if _, err := os.Stat(ociConfigPath); os.IsNotExist(err) {
		return fmt.Errorf("OCI config file not found at %s", ociConfigPath)
	}

	// load the configuration using the OCI SDK
	configProvider, err := common.ConfigurationProviderFromFile(ociConfigPath, "")
	if err != nil {
		return fmt.Errorf("failed to load OCI configuration: %w", err)
	}

	// get the tenancy OCID
	tenancyOCID, err := configProvider.TenancyOCID()
	if err != nil {
		return fmt.Errorf("failed to get tenancy OCID: %w", err)
	} else if tenancyOCID == "" {
		return fmt.Errorf("tenancy OCID not found in OCI config")
	}

	// get the region
	region, err := configProvider.Region()
	if err != nil {
		return fmt.Errorf("failed to get region: %w", err)
	} else if region == "" {
		return fmt.Errorf("region not found in OCI config")
	}

	// read the config file to get the compartment OCID
	cfg, err := ini.Load(ociConfigPath)
	if err != nil {
		return fmt.Errorf("failed to read OCI config file: %w", err)
	}

	// get the compartment OCID from the DEFAULT section
	compartmentOCID := cfg.Section("DEFAULT").Key("compartment_id").String()
	if compartmentOCID == "" {
		// If no compartment_id is specified, use the tenancy OCID as the root compartment
		compartmentOCID = tenancyOCID
	} else if compartmentOCID == "" {
		return fmt.Errorf("compartment OCID not found in OCI config")
	}

	// update struct fields with values from config
	if i.TenancyOCID == "" {
		i.TenancyOCID = tenancyOCID
	}
	if i.CompartmentOCID == "" {
		i.CompartmentOCID = compartmentOCID
	}
	if i.Region == "" {
		i.Region = region
	}

	return nil
}

// createDNSLabel creates a valid DNS label that meets OCI requirements:
// - Must be 15 characters or less
// - Must contain only lowercase alphanumeric characters
// - Maintains uniqueness by using parts of the original name
func createDNSLabel(name string) string {
	// convert to lowercase
	dnsLabel := strings.ToLower(name)

	// If longer than 15 chars, take first 7 and last 7 with 'x' in middle
	if len(dnsLabel) > 15 {
		dnsLabel = dnsLabel[:7] + "x" + dnsLabel[len(dnsLabel)-7:]
	}

	// Replace any non-alphanumeric chars with 'x'
	dnsLabel = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return 'x'
	}, dnsLabel)

	return dnsLabel
}

// getAvailabilityDomainName returns the full name of the first availability domain in the region
func (i *KubernetesRuntimeInfraOKE) getAvailabilityDomainName() (string, error) {
	// create a new identity client
	configProvider := common.DefaultConfigProvider()
	identityClient, err := identity.NewIdentityClientWithConfigurationProvider(configProvider)
	if err != nil {
		return "", fmt.Errorf("failed to create identity client: %w", err)
	}

	// set the region for the client
	identityClient.SetRegion(i.Region)

	// create a request to list availability domains
	request := identity.ListAvailabilityDomainsRequest{
		CompartmentId: common.String(i.CompartmentOCID),
	}

	// call the API to get availability domains
	response, err := identityClient.ListAvailabilityDomains(context.Background(), request)
	if err != nil {
		return "", fmt.Errorf("failed to list availability domains: %w", err)
	}

	// check if we have any availability domains
	if len(response.Items) == 0 {
		return "", fmt.Errorf("no availability domains found in region %s", i.Region)
	}

	// return the name of the first availability domain
	return *response.Items[0].Name, nil
}

// getServiceGatewayServiceID returns the OCI service ID for the service gateway in a given region.
// This ID is used to identify the Oracle Services Network in the service gateway.
func getServiceGatewayServiceID(region string, compartmentID string) (string, error) {
	// create a new virtual network client
	configProvider := common.DefaultConfigProvider()
	vcnClient, err := ocicore.NewVirtualNetworkClientWithConfigurationProvider(configProvider)
	if err != nil {
		return "", fmt.Errorf("failed to create virtual network client: %w", err)
	}

	// set the region for the client
	vcnClient.SetRegion(region)

	// create a request to list services
	request := ocicore.ListServicesRequest{}

	// call the API to get services
	response, err := vcnClient.ListServices(context.Background(), request)
	if err != nil {
		return "", fmt.Errorf("failed to list services: %w", err)
	}

	// find the Oracle Services Network service
	for _, service := range response.Items {
		if service.Name != nil && strings.Contains(*service.Name, "Services In Oracle Services Network") {
			return *service.Id, nil
		}
	}

	// If service not found, return an error
	return "", fmt.Errorf("Oracle Services Network service not found in region %s", region)
}

// getLatestOKEVersion returns the latest Kubernetes version available in OKE
func (i *KubernetesRuntimeInfraOKE) getLatestOKEVersion() (string, error) {
	// create a new container engine client
	configProvider := common.DefaultConfigProvider()
	containerClient, err := ocicontainerengine.NewContainerEngineClientWithConfigurationProvider(configProvider)
	if err != nil {
		return "", fmt.Errorf("failed to create container engine client: %w", err)
	}

	// set the region for the client
	containerClient.SetRegion(i.Region)

	// create a request to list node pool options
	request := ocicontainerengine.GetNodePoolOptionsRequest{
		CompartmentId:    common.String(i.CompartmentOCID),
		NodePoolOptionId: common.String("all"),
	}

	// call the API to get node pool options
	response, err := containerClient.GetNodePoolOptions(context.Background(), request)
	if err != nil {
		return "", fmt.Errorf("failed to get node pool options: %w", err)
	}

	// check if we have any images
	if len(response.Sources) == 0 {
		return "", fmt.Errorf("no OKE worker node images found")
	}

	// find the latest version by parsing version strings
	latestVersion := ""
	for _, source := range response.Sources {
		if sourceType, ok := source.(ocicontainerengine.NodeSourceViaImageOption); ok {
			name := *sourceType.SourceName
			// extract version from name (e.g., "OKE-1.30.10")
			if strings.Contains(name, "OKE-") {
				version := strings.Split(name, "OKE-")[1]
				version = strings.Split(version, "-")[0] // remove any trailing parts
				if latestVersion == "" || version > latestVersion {
					latestVersion = version
				}
			}
		}
	}

	if latestVersion == "" {
		return "", fmt.Errorf("could not determine latest OKE version")
	}

	latestVersion = "v" + latestVersion

	return latestVersion, nil
}

// getOKEWorkerNodeImageOCID returns the OCID of the OKE worker node image
// with version specified in struct
func (i *KubernetesRuntimeInfraOKE) getOKEWorkerNodeImageOCID() (string, error) {
	// create a new container engine client
	configProvider := common.DefaultConfigProvider()
	containerClient, err := ocicontainerengine.NewContainerEngineClientWithConfigurationProvider(configProvider)
	if err != nil {
		return "", fmt.Errorf("failed to create container engine client: %w", err)
	}

	// set the region for the client
	containerClient.SetRegion(i.Region)

	// create a request to list node pool options
	request := ocicontainerengine.GetNodePoolOptionsRequest{
		CompartmentId:    common.String(i.CompartmentOCID),
		NodePoolOptionId: common.String("all"),
	}

	// call the API to get node pool options
	response, err := containerClient.GetNodePoolOptions(context.Background(), request)
	if err != nil {
		return "", fmt.Errorf("failed to get node pool options: %w", err)
	}

	// check if we have any images
	if len(response.Sources) == 0 {
		return "", fmt.Errorf("no OKE worker node images found")
	}

	// find an image with the specified Kubernetes version
	for _, source := range response.Sources {
		// try to get the concrete type
		if sourceType, ok := source.(ocicontainerengine.NodeSourceViaImageOption); ok {
			name := *sourceType.SourceName
			// remove leading 'v' from version for image search
			versionWithoutV := strings.TrimPrefix(i.Version, "v")
			if strings.Contains(name, fmt.Sprintf("OKE-%s", versionWithoutV)) &&
				strings.Contains(name, "aarch64") {
				return *sourceType.ImageId, nil
			}
		}
	}

	return "", fmt.Errorf("no suitable OKE worker node images found with aarch64 architecture and Kubernetes version %s", i.Version)
}

// setupPulumiWorkspace sets up the Pulumi workspace and environment for OKE operations
func (i *KubernetesRuntimeInfraOKE) setupPulumiWorkspace(program pulumi.RunFunc) (auto.Stack, error) {

	// set up state directory
	if err := i.setStateDir(); err != nil {
		return auto.Stack{}, fmt.Errorf("failed to set state directory: %w", err)
	}

	// set environment variables for Pulumi configuration
	if err := i.setPulumiEnvVars(); err != nil {
		return auto.Stack{}, fmt.Errorf("failed to set Pulumi environment variables: %w", err)
	}

	// load OCI configuration
	if err := i.loadOCIConfig(); err != nil {
		return auto.Stack{}, fmt.Errorf("failed to load OCI configuration: %w", err)
	}

	// create Pulumi.yaml project file
	pulumiYaml := `name: oke
runtime: go
description: Oracle Kubernetes Engine (OKE) cluster for Threeport
`
	pulumiYamlPath := filepath.Join(i.stateDir, "Pulumi.yaml")
	if err := os.WriteFile(pulumiYamlPath, []byte(pulumiYaml), 0644); err != nil {
		return auto.Stack{}, fmt.Errorf("failed to create Pulumi.yaml: %w", err)
	}

	ctx := context.Background()

	// create a new workspace with local state backend
	workspace, err := auto.NewLocalWorkspace(
		ctx,
		auto.Program(program),
		auto.WorkDir(i.stateDir),
	)
	if err != nil {
		return auto.Stack{}, fmt.Errorf("failed to create workspace: %w", err)
	}

	// create or select a stack with fully qualified name
	stack, err := auto.UpsertStack(ctx, i.getStackName(), workspace)
	if err != nil {
		return auto.Stack{}, fmt.Errorf("failed to create/select stack: %w", err)
	}

	// set up stack configuration
	err = stack.SetConfig(ctx, "oci:region", auto.ConfigValue{Value: i.Region})
	if err != nil {
		return auto.Stack{}, fmt.Errorf("failed to set region config: %w", err)
	}

	return stack, nil
}

// GetStackState returns the state of the OKE stack as a JSON object
func (i *KubernetesRuntimeInfraOKE) GetStackState() (*datatypes.JSON, error) {

	// set up state directory
	if err := i.setStateDir(); err != nil {
		return nil, fmt.Errorf("failed to set state directory: %w", err)
	}

	// set environment variables for Pulumi configuration
	if err := i.setPulumiEnvVars(); err != nil {
		return nil, fmt.Errorf("failed to set Pulumi environment variables: %w", err)
	}

	ctx := context.Background()

	// create a new workspace with local state backend
	workspace, err := auto.NewLocalWorkspace(
		ctx,
		auto.WorkDir(i.stateDir),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create workspace: %w", err)
	}

	// load stack from workspace
	stack, err := auto.SelectStack(ctx, i.getStackName(), workspace)
	if err != nil {
		return nil, fmt.Errorf("failed to select stack: %w", err)
	}

	// get the stack's state
	state, err := stack.Export(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to export stack state: %w", err)
	}

	// convert state to JSON
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal state to JSON: %w", err)
	}

	jsonState := datatypes.JSON(stateJSON)
	return &jsonState, nil
}

// SetStackState sets the state of the OKE stack from a JSON object
func (i *KubernetesRuntimeInfraOKE) SetStackState(state *datatypes.JSON) error {

	// set up state directory
	if err := i.setStateDir(); err != nil {
		return fmt.Errorf("failed to set state directory: %w", err)
	}

	// set environment variables for Pulumi configuration
	if err := i.setPulumiEnvVars(); err != nil {
		return fmt.Errorf("failed to set Pulumi environment variables: %w", err)
	}

	ctx := context.Background()

	// create a new workspace with local state backend
	workspace, err := auto.NewLocalWorkspace(
		ctx,
		auto.WorkDir(i.stateDir),
	)
	if err != nil {
		return fmt.Errorf("failed to create workspace: %w", err)
	}

	// create/select stack
	stack, err := auto.UpsertStack(ctx, i.getStackName(), workspace)
	if err != nil {
		return fmt.Errorf("failed to create/select stack: %w", err)
	}

	// unmarshal state
	var pulumiState apitype.UntypedDeployment
	err = json.Unmarshal(*state, &pulumiState)
	if err != nil {
		return fmt.Errorf("failed to unmarshal state from JSON: %w", err)
	}

	// set the stack's state and persist to disk
	err = stack.Import(ctx, pulumiState)
	if err != nil {
		return fmt.Errorf("failed to import stack state: %w", err)
	}

	return nil
}

// setPulumiEnvVars sets the environment variables for Pulumi
func (i *KubernetesRuntimeInfraOKE) setPulumiEnvVars() error {
	os.Setenv("PULUMI_BACKEND_URL", "file://"+i.stateDir)
	os.Setenv("PULUMI_HOME", i.stateDir)
	os.Setenv("PULUMI_ORGANIZATION", "organization") // TODO: update these?
	os.Setenv("PULUMI_PROJECT", "oke")
	os.Setenv("PULUMI_CONFIG_PASSPHRASE", "threeport")

	// set plugin path to the default location
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	defaultPluginPath := filepath.Join(userHomeDir, ".pulumi", "plugins")
	os.Setenv("PULUMI_PLUGIN_PATH", defaultPluginPath)

	return nil
}

// setStateDir sets the state directory for the OKE stack
func (i *KubernetesRuntimeInfraOKE) setStateDir() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	i.stateDir = filepath.Join(homeDir, ".threeport", "pulumi-state", i.RuntimeInstanceName)

	// ensure state directory exists
	if err := os.MkdirAll(i.stateDir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	return nil
}

// getStackName returns the name of the OKE stack
func (i *KubernetesRuntimeInfraOKE) getStackName() string {
	return fmt.Sprintf("organization/oke/%s", i.RuntimeInstanceName)
}
