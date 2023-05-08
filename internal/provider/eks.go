package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/nukleros/eks-cluster/pkg/resource"
	"sigs.k8s.io/aws-iam-authenticator/pkg/token"

	"github.com/threeport/threeport/internal/cli"
	"github.com/threeport/threeport/internal/kube"
	"github.com/threeport/threeport/internal/threeport"
)

type ControlPlaneInfraEKS struct {
	ThreeportInstanceName string
	AWSConfigEnv          bool   `yaml:"AWSConfigEnv"`
	AWSConfigProfile      string `yaml:"AWSConfigProfile"`
	AWSRegion             string `yaml:"AWSRegion"`
	AWSAccountID          string `yaml:"AWSAccountID"`
}

// Create installs a Kubernetes cluster using AWS EKS for the threeport control
// plane.
func (i *ControlPlaneInfraEKS) Create(providerConfigDir string) (*kube.KubeConnectionInfo, error) {
	// create a new resource config to configure Kubernetes cluster
	resourceConfig := resource.NewResourceConfig()
	resourceConfig.Name = ThreeportClusterName(i.ThreeportInstanceName)
	resourceConfig.AWSAccountID = i.AWSAccountID
	resourceConfig.InstanceTypes = []string{"t2.medium"}
	resourceConfig.InitialNodes = int32(2)
	resourceConfig.MinNodes = int32(2)
	resourceConfig.MaxNodes = int32(6)
	resourceConfig.DNSManagement = true
	dnsManagementSvcAcct := resource.DNSManagementServiceAccount{
		Name:      threeport.DNSManagerServiceAccountName,
		Namespace: threeport.DNSManagerServiceAccountNamepace,
	}
	resourceConfig.DNSManagementServiceAccount = dnsManagementSvcAcct
	resourceConfig.ClusterAutoscaling = true
	clusterAutoscalingSvcAcct := resource.ClusterAutoscalingServiceAccount{
		Name:      threeport.ClusterAutoscalerServiceAccountName,
		Namespace: threeport.ClusterAutoscalerServiceAccountNamespace,
	}
	resourceConfig.ClusterAutoscalingServiceAccount = clusterAutoscalingSvcAcct
	storageManagementSvcAcct := resource.StorageManagementServiceAccount{
		Name:      threeport.StorageManagerServiceAccountName,
		Namespace: threeport.StorageManagerServiceAccountNamespace,
	}
	resourceConfig.StorageManagementServiceAccount = storageManagementSvcAcct

	// create an AWS config to connect to AWS API
	awsConfig, err := resource.LoadAWSConfig(
		i.AWSConfigEnv,
		i.AWSConfigProfile,
		i.AWSRegion,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS configuration: %w", err)
	}

	// create a resource client for spinning up AWS resources and getting status
	// messages back as it progresses
	msgChan := make(chan string)
	go outputMessages(&msgChan)
	ctx := context.Background()

	// create cluster resources in AWS
	resourceClient := resource.ResourceClient{&msgChan, ctx, awsConfig}
	inventory, err := resourceClient.CreateResourceStack(resourceConfig)
	if err != nil {
		// delete resources that were created, if any
		if deleteErr := resourceClient.DeleteResourceStack(inventory); deleteErr != nil {
			return nil, fmt.Errorf("\nerror creating AWS resources: %w\nerror deleting AWS resources: %s", err, deleteErr)
		}
		return nil, fmt.Errorf("error creating AWS resources: %w", err)
	}

	// write inventory file
	inventoryJSON, err := json.MarshalIndent(inventory, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal AWS inventory: %w", err)
	}
	ioutil.WriteFile(inventoryFilepath(providerConfigDir, i.ThreeportInstanceName), inventoryJSON, 0644)

	kubeConnInfo, err := getConnectionInfo(awsConfig, ThreeportClusterName(i.ThreeportInstanceName))
	if err != nil {
		return nil, fmt.Errorf("failed to EKS cluster connection info: %w", err)
	}

	return kubeConnInfo, nil

}

// Delete deletes an AWS EKS cluster and the threeport control plane with it.
func (i *ControlPlaneInfraEKS) Delete(providerConfigDir string) error {
	// load inventory file
	var resourceInventory resource.ResourceInventory
	inventoryFile := inventoryFilepath(providerConfigDir, i.ThreeportInstanceName)
	inventoryJSON, err := ioutil.ReadFile(inventoryFile)
	if err != nil {
		return fmt.Errorf("failed to read inventory file for EKS resources: %w", err)
	}
	json.Unmarshal(inventoryJSON, &resourceInventory)

	// create an AWS config to connect to AWS API
	awsConfig, err := resource.LoadAWSConfig(
		i.AWSConfigEnv,
		i.AWSConfigProfile,
		i.AWSRegion,
	)
	if err != nil {
		return fmt.Errorf("failed to load AWS configuration: %w", err)
	}

	// create a resource client for spinning up AWS resources and getting status
	// messages back as it progresses
	msgChan := make(chan string)
	go outputMessages(&msgChan)
	ctx := context.Background()

	// create cluster resources in AWS
	resourceClient := resource.ResourceClient{&msgChan, ctx, awsConfig}
	if err := resourceClient.DeleteResourceStack(&resourceInventory); err != nil {
		return fmt.Errorf("error deleting AWS resources: %w", err)
	}

	// TODO: delete inventory file

	return nil
}

// getConnectionInfo queries AWS for the connection token and returns the
// connection info for a particular cluster name.
func getConnectionInfo(awsConfig *aws.Config, clusterName string) (*kube.KubeConnectionInfo, error) {
	svc := eks.NewFromConfig(*awsConfig)

	// get EKS cluster info
	describeClusterinput := &eks.DescribeClusterInput{
		Name: aws.String(clusterName),
	}
	describeClusterResult, err := svc.DescribeCluster(context.TODO(), describeClusterinput)
	if err != nil {
		return nil, fmt.Errorf("failed to describe EKS cluster: %w", err)
	}
	cluster := describeClusterResult.Cluster

	// get EKS cluster token and CA certificate
	gen, err := token.NewGenerator(true, false)
	if err != nil {
		return nil, fmt.Errorf("failed to generate new token: %w", err)
	}
	opts := &token.GetTokenOptions{
		ClusterID: clusterName,
	}
	token, err := gen.GetWithOptions(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get token with options: %w", err)
	}
	ca, err := base64.StdEncoding.DecodeString(*cluster.CertificateAuthority.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode CA data: %w", err)
	}

	kubeConnInfo := kube.KubeConnectionInfo{
		APIEndpoint:   *cluster.Endpoint,
		CACertificate: string(ca),
		EKSToken:      token.Token,
	}

	return &kubeConnInfo, nil
}

// inventoryFilepath returns a standardized filename and path for the EKS
// inventory file.
func inventoryFilepath(providerConfigDir, instanceName string) string {
	inventoryFilename := fmt.Sprintf("eks-inventory-%s.json", instanceName)
	return filepath.Join(providerConfigDir, inventoryFilename)
}

// outputMessages prints the output messages from the resource client to the
// terminal for the CLI user.
func outputMessages(msgChan *chan string) {
	for {
		msg := <-*msgChan
		cli.Info(msg)
	}
}
