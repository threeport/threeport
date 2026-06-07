package provider

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/common"
	ocicontainerengine "github.com/oracle/oci-go-sdk/v65/containerengine"
	ocicore "github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"
	"github.com/pulumi/pulumi-oci/sdk/v3/go/oci"
	"github.com/pulumi/pulumi-oci/sdk/v3/go/oci/containerengine"
	"github.com/pulumi/pulumi-oci/sdk/v3/go/oci/core"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	kube "github.com/threeport/threeport/pkg/kube/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
	yaml "sigs.k8s.io/yaml"
)

// DefaultOKEKubernetesVersion is the default Kubernetes version for OKE clusters.
const DefaultOKEKubernetesVersion = "v1.32.1"

// ociPluginVersion must match github.com/pulumi/pulumi-oci/sdk version in go.mod
const ociPluginVersion = "3.9.0"

// network CIDR blocks for OKE cluster networking
const (
	vcnCidrBlock                = "10.0.0.0/16"
	publicSubnetCidrBlock       = "10.0.0.0/28"
	privateSubnetCidrBlock      = "10.0.10.0/24"
	loadBalancerSubnetCidrBlock = "10.0.20.0/24"
	podsCidrBlock               = "10.244.0.0/16"
	servicesCidrBlock           = "10.96.0.0/16"
)

// KubernetesRuntimeInfraOKE represents the infrastructure for a threeport-managed OKE
// (Oracle Kubernetes Engine) cluster.
type KubernetesRuntimeInfraOKE struct {
	// PulumiWorkspace provides all Pulumi workspace, stack, and state management.
	PulumiWorkspace

	// Kubernetes version of the OKE cluster.
	Version string

	// The Oracle Cloud compartment ID where resources will be created.
	// For genesis bootstrap, this starts as the tenancy OCID and is overwritten
	// with the genesis compartment OCID after createOCICompartment runs.
	// For workload clusters, this starts as the parent compartment OCID
	// (genesis compartment) and is overwritten with the child compartment OCID.
	CompartmentOCID string

	// The Oracle Cloud config provider.
	ConfigProvider common.ConfigurationProvider

	// The Oracle Cloud region where resources will be created.
	Region string

	// The Oracle Cloud shape used for the worker nodes.
	WorkerNodeShape string

	// The number of nodes initially created for the worker node pool.
	WorkerNodeInitialCount int32

	// Service user credentials for OCI operations
	ServiceUserOCID string
	PrivateKeyPEM   string
	PublicKeyPEM    string
	Fingerprint     string
}

// Create installs a Kubernetes cluster using Oracle Cloud OKE for threeport workloads.
// It creates IAM resources and then provisions infrastructure.
func (i *KubernetesRuntimeInfraOKE) Create() (*kube.KubeConnectionInfo, error) {
	// create OCI IAM resources (user, group, policy, API key)
	if err := i.CreateIAM(); err != nil {
		return nil, fmt.Errorf("failed to create OCI IAM resources: %w", err)
	}

	// reset CompartmentOCID to tenancy — CreateIAM overwrites it with the
	// child compartment, but CreateInfra needs the parent (tenancy) so it
	// can idempotently find or create the compartment under it
	tenancyOCID, err := i.ConfigProvider.TenancyOCID()
	if err != nil {
		return nil, fmt.Errorf("failed to get tenancy OCID: %w", err)
	}
	i.CompartmentOCID = tenancyOCID

	return i.CreateInfra()
}

// CreateInfra creates the compartment and provisions OKE cluster infrastructure via Pulumi.
// It does not create IAM resources — call CreateIAM() first for the bootstrap path,
// or set ServiceUserOCID/Fingerprint/PrivateKeyPEM directly for the controller path.
func (i *KubernetesRuntimeInfraOKE) CreateInfra() (*kube.KubeConnectionInfo, error) {
	// derive tenancy OCID from config provider for Pulumi provider configuration
	tenancyOCID, err := i.ConfigProvider.TenancyOCID()
	if err != nil {
		return nil, fmt.Errorf("failed to get tenancy OCID from config provider: %w", err)
	}

	// create compartment for resource isolation
	identityClient, err := identity.NewIdentityClientWithConfigurationProvider(i.ConfigProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to create identity client: %w", err)
	}

	// get home region for compartment creation (compartments must be created in home region)
	homeRegion, err := i.getHomeRegion(identityClient)
	if err != nil {
		return nil, fmt.Errorf("failed to get home region: %w", err)
	}
	identityClient.SetRegion(homeRegion)

	if err := i.createOCICompartment(identityClient); err != nil {
		return nil, fmt.Errorf("failed to create compartment: %w", err)
	}

	// set up Pulumi workspace and get stack
	stack, err := i.SetupStack(func(ctx *pulumi.Context) error {

		// get the availability domain name
		availabilityDomain, err := i.getAvailabilityDomainName()
		if err != nil {
			return fmt.Errorf("failed to get availability domain: %w", err)
		}

		// create OCI provider with explicit configuration using service user credentials
		ociProvider, err := oci.NewProvider(ctx, "oci-provider", &oci.ProviderArgs{
			Region:      pulumi.String(i.Region),
			TenancyOcid: pulumi.String(tenancyOCID),
			UserOcid:    pulumi.String(i.ServiceUserOCID),
			Fingerprint: pulumi.String(i.Fingerprint),
			PrivateKey:  pulumi.String(i.PrivateKeyPEM),
		}, pulumi.Version(ociPluginVersion))
		if err != nil {
			return fmt.Errorf("failed to create OCI provider: %w", err)
		}

		// create VCN for the cluster
		vcn, err := core.NewVcn(ctx, fmt.Sprintf("%s-vcn", i.RuntimeInstanceName), &core.VcnArgs{
			CompartmentId: pulumi.String(i.CompartmentOCID),
			CidrBlock:     pulumi.String(vcnCidrBlock),
			DisplayName:   pulumi.String(fmt.Sprintf("%s-vcn", i.RuntimeInstanceName)),
			DnsLabel:      pulumi.String(createDNSLabel(i.RuntimeInstanceName)),
			IsIpv6enabled: pulumi.Bool(false),
			FreeformTags:  pulumi.StringMap{"threeport-instance": pulumi.String(i.RuntimeInstanceName)},
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
			FreeformTags:  pulumi.StringMap{"threeport-instance": pulumi.String(i.RuntimeInstanceName)},
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
			FreeformTags:  pulumi.StringMap{"threeport-instance": pulumi.String(i.RuntimeInstanceName)},
		}, pulumi.Provider(ociProvider),
			pulumi.DependsOn([]pulumi.Resource{vcn}))
		if err != nil {
			return fmt.Errorf("failed to create NAT Gateway: %w", err)
		}

		// get the service gateway service ID and cidr block
		serviceID, serviceCidrBlock, err := i.getServiceGatewayServiceIdAndCidrBlock()
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
			FreeformTags: pulumi.StringMap{"threeport-instance": pulumi.String(i.RuntimeInstanceName)},
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
			FreeformTags: pulumi.StringMap{"threeport-instance": pulumi.String(i.RuntimeInstanceName)},
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
					Destination:     pulumi.String(serviceCidrBlock),
					DestinationType: pulumi.String("SERVICE_CIDR_BLOCK"),
					NetworkEntityId: serviceGateway.ID(),
				},
			},
			FreeformTags: pulumi.StringMap{"threeport-instance": pulumi.String(i.RuntimeInstanceName)},
		}, pulumi.Provider(ociProvider),
			pulumi.DependsOn([]pulumi.Resource{natGateway, serviceGateway}))
		if err != nil {
			return fmt.Errorf("failed to create private route table: %w", err)
		}

		// define subnets to be used by security lists, cluster, and load balancer
		// subnet CIDR blocks are defined as package-level constants

		// create security list for public subnet
		publicSecList, err := core.NewSecurityList(ctx, fmt.Sprintf("%s-public-seclist", i.RuntimeInstanceName), &core.SecurityListArgs{
			CompartmentId: pulumi.String(i.CompartmentOCID),
			VcnId:         vcn.ID(),
			DisplayName:   pulumi.String(fmt.Sprintf("%s-public-seclist", i.RuntimeInstanceName)),
			FreeformTags:  pulumi.StringMap{"threeport-instance": pulumi.String(i.RuntimeInstanceName)},
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
					Destination:     pulumi.String(serviceCidrBlock),
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
			FreeformTags:  pulumi.StringMap{"threeport-instance": pulumi.String(i.RuntimeInstanceName)},
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
					Destination:     pulumi.String(serviceCidrBlock),
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
			FreeformTags:  pulumi.StringMap{"threeport-instance": pulumi.String(i.RuntimeInstanceName)},
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
			FreeformTags:           pulumi.StringMap{"threeport-instance": pulumi.String(i.RuntimeInstanceName)},
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
			FreeformTags:           pulumi.StringMap{"threeport-instance": pulumi.String(i.RuntimeInstanceName)},
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
			FreeformTags:           pulumi.StringMap{"threeport-instance": pulumi.String(i.RuntimeInstanceName)},
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
			FreeformTags:      pulumi.StringMap{"threeport-instance": pulumi.String(i.RuntimeInstanceName)},
			EndpointConfig: &containerengine.ClusterEndpointConfigArgs{
				IsPublicIpEnabled: pulumi.Bool(true),
				SubnetId:          publicSubnet.ID(),
				NsgIds:            pulumi.StringArray{}, // optional: Add network security group IDs if needed
			},
			Options: &containerengine.ClusterOptionsArgs{
				KubernetesNetworkConfig: &containerengine.ClusterOptionsKubernetesNetworkConfigArgs{
					PodsCidr:     pulumi.String(podsCidrBlock),
					ServicesCidr: pulumi.String(servicesCidrBlock),
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
			FreeformTags:      pulumi.StringMap{"threeport-instance": pulumi.String(i.RuntimeInstanceName)},
			InitialNodeLabels: containerengine.NodePoolInitialNodeLabelArray{
				&containerengine.NodePoolInitialNodeLabelArgs{
					Key:   pulumi.String(kube.ThreeportManagedByLabelKey),
					Value: pulumi.String(kube.ThreeportManagedByLabelValue),
				},
			},
			NodeConfigDetails: &containerengine.NodePoolNodeConfigDetailsArgs{
				Size: pulumi.Int(i.WorkerNodeInitialCount),
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
	_, err = i.RunUp(ctx, stack)
	if err != nil {
		return nil, fmt.Errorf("failed to deploy stack: %w", err)
	}

	return i.GetConnection()
}

// DeployInfra creates the compartment and provisions OKE cluster infrastructure
// via Pulumi, discarding the connection info. This satisfies the InfraProvider
// interface for use with the shared infrastructure lifecycle abstraction.
func (i *KubernetesRuntimeInfraOKE) DeployInfra() error {
	_, err := i.CreateInfra()
	return err
}

// DestroyInfra destroys the OKE cluster infrastructure via Pulumi. This
// satisfies the InfraProvider interface for use with the shared infrastructure
// lifecycle abstraction.
func (i *KubernetesRuntimeInfraOKE) DestroyInfra() error {
	return i.Delete()
}

// Delete deletes an Oracle Cloud OKE cluster.
func (i *KubernetesRuntimeInfraOKE) Delete() error {
	return i.DestroyStack()
}

// GetClusterOCID gets the OCID of the OKE cluster.
func (i *KubernetesRuntimeInfraOKE) GetClusterOCID(okeClusterName string) (string, error) {

	containerClient, err := ocicontainerengine.NewContainerEngineClientWithConfigurationProvider(i.ConfigProvider)
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
	// create a new container engine client
	clusterOCID, err := i.GetClusterOCID(i.RuntimeInstanceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster OCID: %w", err)
	}

	containerClient, err := ocicontainerengine.NewContainerEngineClientWithConfigurationProvider(i.ConfigProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to create container engine client: %w", err)
	}

	// set the region for the client
	containerClient.SetRegion(i.Region)

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

	token, tokenExpirationTime, err := util.GenerateOkeToken(clusterOCID, i.ConfigProvider)
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
			CertificateAuthorityData string `json:"certificate-authority-data"`
		} `json:"cluster"`
	} `json:"clusters"`
}

// loadOCIConfig reads the OCI configuration using the OCI SDK and
// populates KubernetesRuntimeInfraOKE struct fields
func (i *KubernetesRuntimeInfraOKE) LoadOCIConfig(
	region,
	ociConfigProfile,
	compartmentOCID string,
) error {
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
	configProvider, err := common.ConfigurationProviderFromFileWithProfile(
		ociConfigPath,
		ociConfigProfile,
		"",
	)
	if err != nil {
		return fmt.Errorf("failed to load OCI configuration: %w", err)
	}
	i.ConfigProvider = configProvider

	// validate tenancy OCID is available
	tenancyOCID, err := configProvider.TenancyOCID()
	if err != nil {
		return fmt.Errorf("failed to get tenancy OCID: %w", err)
	} else if tenancyOCID == "" {
		return fmt.Errorf("tenancy OCID not found in OCI config")
	}

	// set the region if it is provided via cli flag,
	// otherwise get the region from the OCI config
	if region != "" {
		i.Region = region
	} else {
		i.Region, err = configProvider.Region()
		if err != nil {
			return fmt.Errorf("failed to get region: %w", err)
		} else if i.Region == "" {
			return fmt.Errorf("region not found in OCI config")
		}
	}

	// set the compartment OCID if it is provided via cli flag,
	// otherwise use the tenancy OCID as the root compartment
	if compartmentOCID != "" {
		i.CompartmentOCID = compartmentOCID
	} else {
		i.CompartmentOCID = tenancyOCID
	}

	// set stack configs now that region is resolved
	i.StackConfigs = map[string]string{"oci:region": i.Region}

	return nil
}

// LoadServiceCredentialsFromConfig populates ServiceUserOCID, Fingerprint, and
// PrivateKeyPEM from the already-loaded OCI ConfigProvider. Used in the
// control-plane-only path where IAM bootstrap is skipped.
func (i *KubernetesRuntimeInfraOKE) LoadServiceCredentialsFromConfig() error {
	// get user OCID from config provider
	userOCID, err := i.ConfigProvider.UserOCID()
	if err != nil {
		return fmt.Errorf("failed to get user OCID from config: %w", err)
	}
	i.ServiceUserOCID = userOCID

	// get fingerprint from config provider
	fingerprint, err := i.ConfigProvider.KeyFingerprint()
	if err != nil {
		return fmt.Errorf("failed to get key fingerprint from config: %w", err)
	}
	i.Fingerprint = fingerprint

	// get private key from config provider
	privateKey, err := i.ConfigProvider.PrivateRSAKey()
	if err != nil {
		return fmt.Errorf("failed to get private key from config: %w", err)
	}
	privateKeyBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})
	i.PrivateKeyPEM = string(privateKeyBytes)

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
	identityClient, err := identity.NewIdentityClientWithConfigurationProvider(i.ConfigProvider)
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

// getServiceGatewayServiceIdAndCidrBlock returns the OCI service ID for the service gateway in a given region.
// This ID is used to identify the Oracle Services Network in the service gateway.
func (i *KubernetesRuntimeInfraOKE) getServiceGatewayServiceIdAndCidrBlock() (string, string, error) {
	// create a new virtual network client
	vcnClient, err := ocicore.NewVirtualNetworkClientWithConfigurationProvider(i.ConfigProvider)
	if err != nil {
		return "", "", fmt.Errorf("failed to create virtual network client: %w", err)
	}

	// set the region for the client
	vcnClient.SetRegion(i.Region)

	// create a request to list services
	request := ocicore.ListServicesRequest{}

	// call the API to get services
	response, err := vcnClient.ListServices(context.Background(), request)
	if err != nil {
		return "", "", fmt.Errorf("failed to list services: %w", err)
	}

	// find the Oracle Services Network service
	for _, service := range response.Items {
		if service.Name != nil && strings.Contains(*service.Name, "Services In Oracle Services Network") {
			return *service.Id, *service.CidrBlock, nil
		}
	}

	// If service not found, return an error
	return "", "", fmt.Errorf("Oracle Services Network service not found in region %s", i.Region)
}

// getOKEWorkerNodeImageOCID returns the OCID of the OKE worker node image
// with version specified in struct
func (i *KubernetesRuntimeInfraOKE) getOKEWorkerNodeImageOCID() (string, error) {
	// create a new container engine client
	containerClient, err := ocicontainerengine.NewContainerEngineClientWithConfigurationProvider(i.ConfigProvider)
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

	// determine image architecture from node shape
	arch := "x86_64"
	if strings.Contains(i.WorkerNodeShape, ".A1.") {
		arch = "aarch64"
	}

	// find an image with the specified Kubernetes version and architecture
	// use delimiter after version to avoid partial matches (e.g., 1.32.1 matching 1.32.10)
	versionWithoutV := strings.TrimPrefix(i.Version, "v")
	versionPattern := fmt.Sprintf("OKE-%s-", versionWithoutV)
	for _, source := range response.Sources {
		// try to get the concrete type
		if sourceType, ok := source.(ocicontainerengine.NodeSourceViaImageOption); ok {
			name := *sourceType.SourceName
			if strings.Contains(name, versionPattern) &&
				strings.Contains(name, arch) {
				return *sourceType.ImageId, nil
			}
		}
	}

	return "", fmt.Errorf("no suitable OKE worker node images found with %s architecture and Kubernetes version %s", arch, i.Version)
}

// getHomeRegion queries OCI to get the tenancy's home region for IAM operations.
func (i *KubernetesRuntimeInfraOKE) getHomeRegion(identityClient identity.IdentityClient) (string, error) {
	// use the config file region to make the initial query
	configRegion, err := i.ConfigProvider.Region()
	if err != nil {
		return "", fmt.Errorf("failed to get region from config provider: %w", err)
	}
	identityClient.SetRegion(configRegion)

	// derive tenancy OCID from config provider
	tenancyOCID, err := i.ConfigProvider.TenancyOCID()
	if err != nil {
		return "", fmt.Errorf("failed to get tenancy OCID from config provider: %w", err)
	}

	// query the tenancy to get the home region key
	tenancyRequest := identity.GetTenancyRequest{
		TenancyId: common.String(tenancyOCID),
	}
	tenancyResponse, err := identityClient.GetTenancy(context.Background(), tenancyRequest)
	if err != nil {
		return "", fmt.Errorf("failed to get tenancy details: %w", err)
	}

	if tenancyResponse.Tenancy.HomeRegionKey == nil {
		return "", fmt.Errorf("home region key not found in tenancy response")
	}

	// get the list of regions to map the home region key to full region name
	regionsResponse, err := identityClient.ListRegions(context.Background())
	if err != nil {
		return "", fmt.Errorf("failed to list regions: %w", err)
	}

	// find the region with matching key
	homeRegionKey := *tenancyResponse.Tenancy.HomeRegionKey
	for _, region := range regionsResponse.Items {
		if region.Key != nil && *region.Key == homeRegionKey {
			if region.Name == nil {
				return "", fmt.Errorf("region name not found for key %s", homeRegionKey)
			}
			return *region.Name, nil
		}
	}

	return "", fmt.Errorf("could not find region name for home region key %s", homeRegionKey)
}

// CreateIAM creates OCI IAM resources (user, group, policy, API key) for the threeport instance.
//
// This function uses the OCI SDK (not Pulumi) for identity resource creation because OCI identity
// resources (users, groups, policies, API keys) have propagation delays across OCI's distributed
// services. After creation, these resources must be synchronized across all OCI regional endpoints
// before they can be used for authentication. By using the SDK directly, we can synchronously
// create each resource and then explicitly validate propagation via validateOCIUserPropagation()
// before proceeding to infrastructure deployment.
//
// Pulumi operates asynchronously and cannot wait for cross-service propagation to complete.
// If we used Pulumi for identity resources, subsequent infrastructure operations would fail
// with authentication errors because the service user credentials would not yet be available
// in all required OCI services. The SDK approach ensures the service user is fully operational
// before Pulumi begins deploying VCN, OKE cluster, and other infrastructure resources.
func (i *KubernetesRuntimeInfraOKE) CreateIAM() error {
	fmt.Printf("Creating OCI user and credentials using SDK\n")

	// create a new identity client
	identityClient, err := identity.NewIdentityClientWithConfigurationProvider(i.ConfigProvider)
	if err != nil {
		return fmt.Errorf("failed to create identity client: %w", err)
	}

	// get the home region for IAM operations (users, policies must be created in home region)
	homeRegion, err := i.getHomeRegion(identityClient)
	if err != nil {
		return fmt.Errorf("failed to get home region: %w", err)
	}

	// set the region for the identity client to home region for IAM operations
	identityClient.SetRegion(homeRegion)

	// create compartment before policies — policies reference compartment by name
	if err := i.createOCICompartment(identityClient); err != nil {
		return fmt.Errorf("failed to create compartment: %w", err)
	}

	// create service user
	if err := i.createOCIServiceUser(identityClient); err != nil {
		return fmt.Errorf("failed to create service user: %w", err)
	}

	// generate API key pair
	if err := i.generateOCIAPIKeyPair(); err != nil {
		return fmt.Errorf("failed to generate API key pair: %w", err)
	}

	// create API key for service user
	if err := i.createOCIAPIKey(identityClient); err != nil {
		return fmt.Errorf("failed to create API key: %w", err)
	}

	// create groups
	if err := i.createOCIGroup(identityClient); err != nil {
		return fmt.Errorf("failed to create groups: %w", err)
	}

	// create policies
	if err := i.createOCIPolicy(identityClient); err != nil {
		return fmt.Errorf("failed to create policies: %w", err)
	}

	// add user to groups
	if err := i.addOCIUserToGroup(identityClient); err != nil {
		return fmt.Errorf("failed to add user to groups: %w", err)
	}

	// write OCI configuration files
	if err := i.writeOCIConfiguration(); err != nil {
		return fmt.Errorf("failed to write OCI configuration: %w", err)
	}

	// validate user propagation across all OCI services
	if err := i.validateOCIUserPropagation(); err != nil {
		return fmt.Errorf("failed to validate user propagation: %w", err)
	}

	fmt.Printf("Successfully created OCI user and credentials\n")
	return nil
}

// DeleteCompartment deletes only the OCI compartment for this instance.
// Use this for the controller path where no IAM resources were created.
func (i *KubernetesRuntimeInfraOKE) DeleteCompartment() error {
	identityClient, err := identity.NewIdentityClientWithConfigurationProvider(i.ConfigProvider)
	if err != nil {
		return fmt.Errorf("failed to create identity client: %w", err)
	}

	// get home region for compartment deletion
	homeRegion, err := i.getHomeRegion(identityClient)
	if err != nil {
		return fmt.Errorf("failed to get home region: %w", err)
	}
	identityClient.SetRegion(homeRegion)

	return i.deleteOCICompartment(identityClient)
}

// DeleteOCIResources deletes all OCI resources created for this instance.
func (i *KubernetesRuntimeInfraOKE) DeleteOCIResources() error {
	// create a new identity client
	identityClient, err := identity.NewIdentityClientWithConfigurationProvider(i.ConfigProvider)
	if err != nil {
		return fmt.Errorf("failed to create identity client: %w", err)
	}

	// get the home region for IAM operations (compartments, users, policies must be deleted in home region)
	homeRegion, err := i.getHomeRegion(identityClient)
	if err != nil {
		return fmt.Errorf("failed to get home region: %w", err)
	}

	// set the region for the identity client to home region for IAM operations
	identityClient.SetRegion(homeRegion)

	// delete in dependency order: policy, user (removes group memberships), group, compartment.
	// collect all errors so callers know what failed.
	var errs []error

	if err := i.deleteOCIPolicy(identityClient); err != nil {
		fmt.Printf("Warning: failed to delete OCI policy: %v\n", err)
		errs = append(errs, fmt.Errorf("policy: %w", err))
	}

	if err := i.deleteOCIUser(identityClient); err != nil {
		fmt.Printf("Warning: failed to delete OCI user: %v\n", err)
		errs = append(errs, fmt.Errorf("user: %w", err))
	}

	if err := i.deleteOCIGroup(identityClient); err != nil {
		fmt.Printf("Warning: failed to delete OCI group: %v\n", err)
		errs = append(errs, fmt.Errorf("group: %w", err))
	}

	if err := i.deleteOCICompartment(identityClient); err != nil {
		fmt.Printf("Warning: failed to delete OCI compartment: %v\n", err)
		errs = append(errs, fmt.Errorf("compartment: %w", err))
	}

	// clean up local OCI configuration files
	if err := i.deleteOCIConfiguration(); err != nil {
		fmt.Printf("Warning: failed to clean up OCI configuration: %v\n", err)
		errs = append(errs, fmt.Errorf("config cleanup: %w", err))
	}

	return errors.Join(errs...)
}
