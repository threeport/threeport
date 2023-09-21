package provider

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/nukleros/eks-cluster/pkg/connection"
	"github.com/nukleros/eks-cluster/pkg/resource"
	"gopkg.in/ini.v1"

	kube "github.com/threeport/threeport/pkg/kube/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

// KubernetesRuntimeInfraEKS represents the infrastructure for a threeport-managed EKS
// cluster.
type KubernetesRuntimeInfraEKS struct {
	// The unique name of the kubernetes runtime instance managed by threeport.
	RuntimeInstanceName string

	// The AWS account ID where the cluster infra is provisioned.
	AwsAccountID string

	// The configuration containing credentials to connect to an AWS account.
	AwsConfig *aws.Config

	// The eks-clutser client used to create AWS EKS resources.
	ResourceClient *resource.ResourceClient

	// The inventory of AWS resources used to run an EKS cluster.
	ResourceInventory *resource.ResourceInventory

	// The number of availability zones the eks-cluster will be deployed across.
	ZoneCount int32

	// The AWS isntance type used for the default node group.
	DefaultNodeGroupInstanceType string

	// The number of nodes initially created for the default node group.
	DefaultNodeGroupInitialNodes int32

	// The minimum number of nodes to maintain in the default node group.
	DefaultNodeGroupMinNodes int32

	// The maximum number of nodes allowed in the default node group.
	DefaultNodeGroupMaxNodes int32
}

// Create installs a Kubernetes cluster using AWS EKS for threeport workloads.
func (i *KubernetesRuntimeInfraEKS) Create() (*kube.KubeConnectionInfo, error) {
	// create a new resource config to configure Kubernetes cluster
	resourceConfig := resource.NewResourceConfig()
	resourceConfig.Name = i.RuntimeInstanceName
	resourceConfig.AWSAccountID = i.AwsAccountID
	resourceConfig.DesiredAZCount = i.ZoneCount
	resourceConfig.InstanceTypes = []string{i.DefaultNodeGroupInstanceType}
	resourceConfig.InitialNodes = i.DefaultNodeGroupInitialNodes
	resourceConfig.MinNodes = i.DefaultNodeGroupMinNodes
	resourceConfig.MaxNodes = i.DefaultNodeGroupMaxNodes
	resourceConfig.DNSManagement = true
	resourceConfig.DNS01Challenge = true
	resourceConfig.DNSManagementServiceAccount = resource.DNSManagementServiceAccount{
		Name:      threeport.DNSManagerServiceAccountName,
		Namespace: threeport.DNSManagerServiceAccountNamepace,
	}
	resourceConfig.DNS01ChallengeServiceAccount = resource.DNS01ChallengeServiceAccount{
		Name:      threeport.DNS01ChallengeServiceAccountName,
		Namespace: threeport.DNS01ChallengeServiceAccountNamepace,
	}
	resourceConfig.ClusterAutoscaling = true
	resourceConfig.ClusterAutoscalingServiceAccount = resource.ClusterAutoscalingServiceAccount{
		Name:      threeport.ClusterAutoscalerServiceAccountName,
		Namespace: threeport.ClusterAutoscalerServiceAccountNamespace,
	}
	resourceConfig.StorageManagementServiceAccount = resource.StorageManagementServiceAccount{
		Name:      threeport.StorageManagerServiceAccountName,
		Namespace: threeport.StorageManagerServiceAccountNamespace,
	}
	resourceConfig.Tags = ThreeportProviderTags()

	// create EKS cluster resource stack in AWS
	if err := i.ResourceClient.CreateResourceStack(resourceConfig); err != nil {
		return nil, fmt.Errorf("failed to create eks resource stack: %w", err)
	}

	// get kubernetes API connection info
	eksClusterConn := connection.EKSClusterConnectionInfo{ClusterName: i.RuntimeInstanceName}
	if err := eksClusterConn.Get(i.AwsConfig); err != nil {
		return nil, fmt.Errorf("failed to get EKS cluster connection info: %w", err)
	}
	kubeConnInfo := kube.KubeConnectionInfo{
		APIEndpoint:        eksClusterConn.APIEndpoint,
		CACertificate:      eksClusterConn.CACertificate,
		EKSToken:           eksClusterConn.Token,
		EKSTokenExpiration: eksClusterConn.TokenExpiration,
	}

	return &kubeConnInfo, nil
}

// Delete deletes an AWS EKS cluster.
func (i *KubernetesRuntimeInfraEKS) Delete() error {
	// delete EKS cluster resources
	if err := i.ResourceClient.DeleteResourceStack(i.ResourceInventory); err != nil {
		return fmt.Errorf("failed to delete eks cluster resource stack: %w", err)
	}

	return nil
}

// RefreshConnection gets a new token for authentication to an EKS cluster.
func (i *KubernetesRuntimeInfraEKS) RefreshConnection() (*kube.KubeConnectionInfo, error) {
	// get connection info
	eksClusterConn := connection.EKSClusterConnectionInfo{
		ClusterName: i.RuntimeInstanceName,
	}
	if err := eksClusterConn.Get(i.AwsConfig); err != nil {
		return nil, fmt.Errorf("failed to retrieve EKS cluster connection info for token refresh: %w", err)
	}

	// construct KubeConnectionInfo object
	kubeConnInfo := kube.KubeConnectionInfo{
		APIEndpoint:        eksClusterConn.APIEndpoint,
		CACertificate:      eksClusterConn.CACertificate,
		EKSToken:           eksClusterConn.Token,
		EKSTokenExpiration: eksClusterConn.TokenExpiration,
	}

	return &kubeConnInfo, nil
}

// EKSInventoryFilepath returns a standardized filename and path for the EKS
// inventory file.
func EKSInventoryFilepath(providerConfigDir, instanceName string) string {
	inventoryFilename := fmt.Sprintf("eks-inventory-%s.json", instanceName)
	return filepath.Join(providerConfigDir, inventoryFilename)
}

// GetKeysFromLocalConfig returns the access key ID and secret access key from
// either the environment or local AWS credentials.
func GetKeysFromLocalConfig(profile string) (string, string, error) {
	envAccessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
	envSecretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")

	if envAccessKeyID != "" && envSecretAccessKey != "" {
		return envAccessKeyID, envSecretAccessKey, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("failed to get user home directory: %w", err)
	}
	awsCredentials, err := ini.Load(filepath.Join(homeDir, ".aws", "credentials"))
	if err != nil {
		return "", "", fmt.Errorf("failed to load aws credentials: %w", err)
	}
	var accessKeyID string
	var secretAccessKey string
	if awsCredentials.Section(profile).HasKey("aws_access_key_id") &&
		awsCredentials.Section(profile).HasKey("aws_secret_access_key") {
		accessKeyID = awsCredentials.Section(profile).Key("aws_access_key_id").String()
		secretAccessKey = awsCredentials.Section(profile).Key("aws_secret_access_key").String()
	} else {
		return "", "", errors.New("unable to get AWS credentials from environment or local credentials")
	}

	return accessKeyID, secretAccessKey, nil
}
