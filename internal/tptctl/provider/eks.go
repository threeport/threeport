package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/nukleros/eks-cluster/pkg/resource"

	"github.com/threeport/threeport/internal/tptctl/install"
	"github.com/threeport/threeport/internal/tptctl/output"
)

// CreateControlPlaneOnEKS creates an EKS cluster on AWS and installs the
// threeport control plane.
func (c *ControlPlane) CreateControlPlaneOnEKS(providerConfigDir string) (string, error) {
	var threeportAPIEndpoint string

	// create and configure eks resource config
	resourceConfig := resource.NewResourceConfig()
	resourceConfig.Name = c.ThreeportClusterName()
	resourceConfig.AWSAccountID = c.ProviderAccountID
	resourceConfig.MinNodes = c.MinClusterNodes
	resourceConfig.MaxNodes = c.MaxClusterNodes
	resourceConfig.InstanceTypes = []string{c.DefaultAWSInstanceType}
	resourceConfig.Tags = map[string]string{"provisioner": "tptctl"}
	if c.RootDomainName != "" {
		resourceConfig.DNSManagement = true
		resourceConfig.DNSManagementServiceAccount = resource.DNSManagementServiceAccount{
			Name:      install.SupportServicesDNSManagementServiceAccount,
			Namespace: install.SupportServicesIngressNamespace,
		}
	}

	// create eks resource client
	msgChan := make(chan string)
	go outputMessages(&msgChan)
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return threeportAPIEndpoint, fmt.Errorf("failed to load default config for AWS: %w")
	}
	resourceClient := resource.ResourceClient{&msgChan, ctx, &cfg}

	// create resources in aws
	output.Info("Creating resources for EKS cluster...")
	inventory, createErr := resourceClient.CreateResourceStack(resourceConfig)

	// write inventory file
	// important: write file even if there was some error so we can clean up
	inventoryJSON, err := json.MarshalIndent(inventory, "", " ")
	if err != nil {
		return threeportAPIEndpoint, fmt.Errorf("failed to marshal inventory to JSON: %w")
	}
	ioutil.WriteFile(c.inventoryFilePath(providerConfigDir), inventoryJSON, 0644)

	// handle any resource creation error
	if createErr != nil {
		output.Error("Problem encountered creating resources. Deleting resources that were created...", err)
		if deleteErr := resourceClient.DeleteResourceStack(inventory); deleteErr != nil {
			return threeportAPIEndpoint, fmt.Errorf("\nerror creating resources: %w\nerror deleting resources: %w", err, deleteErr)
		}
		return threeportAPIEndpoint, fmt.Errorf("error creating resources: %w", createErr)
	}

	// update kubeconfig
	updateKubeconfig := exec.Command(
		"aws",
		"eks",
		"update-kubeconfig",
		"--name",
		c.ThreeportClusterName(),
		"--kubeconfig",
		c.kubeconfigFilePath(providerConfigDir),
	)
	updateKubeconfigOut, err := updateKubeconfig.CombinedOutput()
	if err != nil {
		output.Error(fmt.Sprintf("aws eks error: %s", updateKubeconfigOut), nil)
		return threeportAPIEndpoint, fmt.Errorf("failed to update kubeconfig: %w", err)
	}
	output.Info("kubeconfig updated to include new EKS cluster")

	// install support services operator
	loadBalancerURL, err := install.InstallSupportServicesOperator(
		c.kubeconfigFilePath(providerConfigDir),
		inventory.DNSManagementRole.RoleARN,
		c.RootDomainName,
		c.AdminEmail,
	)
	if err != nil {
		return threeportAPIEndpoint, fmt.Errorf("failed to install support services operator on EKS cluster: %w", err)
	}

	// install threeport API
	if err := install.InstallAPI(
		c.kubeconfigFilePath(providerConfigDir), c.ThreeportClusterName(), c.RootDomainName,
		loadBalancerURL,
	); err != nil {
		return threeportAPIEndpoint, fmt.Errorf("failed to install threeport API on EKS cluster: %w", err)
	}
	threeportAPIEndpoint = fmt.Sprintf("https://%s.%s", c.ThreeportClusterName(), c.RootDomainName)

	// install workload controller
	if err := install.InstallWorkloadController(c.kubeconfigFilePath(providerConfigDir)); err != nil {
		return threeportAPIEndpoint, fmt.Errorf("failed to install workload controller on EKS cluster: %w", err)
	}

	return threeportAPIEndpoint, nil
}

// DeleteControlPlaneOnEKS deletes the ingress component of a control plane
// cluster to remove any load balancers and then removes the infra to completely
// destroy an instance of a threeport control plane.
func (c *ControlPlane) DeleteControlPlaneOnEKS(providerConfigDir string) error {
	// delete ingress resource to clean up DNS records
	// we do not return an error here so that the deltion of AWS resources
	// continues
	if err := install.UninstallAPIIngress(c.kubeconfigFilePath(providerConfigDir)); err != nil {
		output.Error("Failed to delete threeport API ingress resource in Kubernetes", err)
		output.Warning("This may result in a dangling Route53 resources in AWS - recommend checking your AWS account")
		output.Info("Continuing with control plane deletion...")
	}

	// delete ingress component to remove cloud load balancer
	// we do not return an error here so that the deltion of AWS resources
	// continues
	if err := install.UninstallIngressComponent(c.kubeconfigFilePath(providerConfigDir)); err != nil {
		output.Error("Failed to delete support services ingress component", err)
		output.Warning("This may result in a dangling load balancer resource in AWS - recommend checking your AWS account")
		output.Info("Continuing with control plane deletion...")
	}

	// get resource inventory
	var resourceInventory resource.ResourceInventory
	inventoryJSON, err := ioutil.ReadFile(c.inventoryFilePath(providerConfigDir))
	if err != nil {
		return fmt.Errorf("failed to read inventory file: %w", err)
	}
	json.Unmarshal(inventoryJSON, &resourceInventory)

	// create eks resource client
	msgChan := make(chan string)
	go outputMessages(&msgChan)
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx)
	resourceClient := resource.ResourceClient{&msgChan, ctx, &cfg}

	// delete resources
	output.Info("Deleting resources for EKS cluster...")
	if err := resourceClient.DeleteResourceStack(&resourceInventory); err != nil {
		return fmt.Errorf("failed to delete EKS resources: %w", err)
	}

	// remove inventory file
	if err := os.Remove(c.inventoryFilePath(providerConfigDir)); err != nil {
		fmt.Errorf("failed to remove inventory file: %w", err)
	}

	// remove kubeconfig
	if err := os.Remove(c.kubeconfigFilePath(providerConfigDir)); err != nil {
		fmt.Errorf("failed to remove kubeconfig file: %w", err)
	}

	return nil
}

// inventoryFilePath returns the default inventory filepath.  The inventory
// contains all the cloud provider IDs for infra created for a threeport control
// plane.
func (c *ControlPlane) inventoryFilePath(providerConfigDir string) string {
	return filepath.Join(
		providerConfigDir,
		fmt.Sprintf("eks-inventory-%s.json", c.ThreeportClusterName()),
	)
}

// kubeconfigFilePath returns a filepath for a kubeconfig to connect to the
// Kubernetes API in a threeport control plane cluster.  This filepath is unique
// to each threeport instance name so that each gets its own distinct file.
func (c *ControlPlane) kubeconfigFilePath(providerConfigDir string) string {
	return filepath.Join(
		providerConfigDir,
		fmt.Sprintf("kubeconfig-%s", c.ThreeportClusterName()),
	)
}

// outputMessages takes messages received on a channel from the eks-cluster
// library as it provisions infra and prints it to the console for the user.
func outputMessages(msgChan *chan string) {
	for {
		msg := <-*msgChan
		output.Info(msg)
	}
}
