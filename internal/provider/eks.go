package provider

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	aws_v1 "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"

	"github.com/nukleros/eks-cluster/pkg/resource"
	"gopkg.in/ini.v1"

	"github.com/threeport/threeport/internal/kube"
	"github.com/threeport/threeport/internal/threeport"
)

// ClusterInfraEKS represents the infrastructure for a threeport-managed EKS
// cluster.
type ClusterInfraEKS struct {
	// The unique name of the threeport instance.
	ThreeportInstanceName string

	// The AWS account ID where the cluster infra is provisioned.
	AwsAccountID string

	// The configuration containing credentials to connect to an AWS account.
	AwsConfig aws.Config

	// The eks-clutser client used to create AWS EKS resources.
	ResourceClient resource.ResourceClient

	// The inventory of AWS resources used to run an EKS cluster.
	ResourceInventory resource.ResourceInventory
}

// Create installs a Kubernetes cluster using AWS EKS for threeport workloads.
func (i *ClusterInfraEKS) Create() (*kube.KubeConnectionInfo, error) {
	// create a new resource config to configure Kubernetes cluster
	resourceConfig := resource.NewResourceConfig()
	resourceConfig.Name = ThreeportClusterName(i.ThreeportInstanceName)
	resourceConfig.AWSAccountID = i.AwsAccountID
	resourceConfig.InstanceTypes = []string{"t2.medium"}
	resourceConfig.InitialNodes = int32(2)
	resourceConfig.MinNodes = int32(2)
	resourceConfig.MaxNodes = int32(6)
	resourceConfig.DNSManagement = true
	resourceConfig.DNSManagementServiceAccount = resource.DNSManagementServiceAccount{
		Name:      threeport.DNSManagerServiceAccountName,
		Namespace: threeport.DNSManagerServiceAccountNamepace,
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
	resourceConfig.Tags = map[string]string{"ProvisionedBy": "tptctl"}

	// create EKS cluster resource stack in AWS
	if err := i.ResourceClient.CreateResourceStack(resourceConfig); err != nil {
		return nil, fmt.Errorf("failed to create eks resource stack: %w", err)
	}

	// get kubernetes API connection info
	kubeConnInfo, err := getEKSConnectionInfo(
		&i.AwsConfig,
		ThreeportClusterName(i.ThreeportInstanceName),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to EKS cluster connection info: %w", err)
	}

	return &kubeConnInfo, nil
}

// Delete deletes an AWS EKS cluster.
// func (i *ControlPlaneInfraEKS) Delete(providerConfigDir string) error {
func (i *ClusterInfraEKS) Delete() error {
	// delete EKS cluster resources
	if err := i.ResourceClient.DeleteResourceStack(i.ResourceInventory); err != nil {
		return fmt.Errorf("failed to delete eks cluster resource stack: %w", err)
	}

	return nil
}

// RefreshConnection gets a new token for authentication to an EKS cluster.
func (i *ClusterInfraEKS) RefreshConnection() (*kube.KubeConnectionInfo, error) {
	return getEKSConnectionInfo(
		&i.AwsConfig,
		ThreeportClusterName(i.ThreeportInstanceName),
	)
}

// getEKSConnectionInfo queries AWS for the connection token and returns the
// connection info for a particular cluster name.
// func getEKSConnectionInfo(awsConfig *aws.Config, awsProfile, clusterName string) (*kube.KubeConnectionInfo, error) {
func getEKSConnectionInfo(awsConfig *aws.Config, clusterName string) (*kube.KubeConnectionInfo, error) {
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

	// construct a config object for the earlier v1 version of AWS SDK
	creds, err := awsConfig.Credentials.Retrieve(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve credentials from AWS config: %w", err)
	}
	v1Config := aws_v1.Config{
		Region: aws_v1.String(awsConfig.Region),
		Credentials: credentials.NewStaticCredentials(
			creds.AccessKeyID,
			creds.SecretAccessKey,
			creds.SessionToken,
		),
	}

	// create a new session using the v1 SDK which is used by
	// sigs.k8s.io/aws-iam-authenticator/pkg/token to get a token
	sessionOpts := session.Options{
		Config:            v1Config,
		SharedConfigState: session.SharedConfigEnable,
	}
	awsSession, err := session.NewSessionWithOptions(sessionOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create new AWS session for generating EKS cluster token: %w", err)
	}

	// get EKS cluster token and CA certificate
	gen, err := token.NewGenerator(true, false)
	if err != nil {
		return nil, fmt.Errorf("failed to generate new token: %w", err)
	}
	opts := &token.GetTokenOptions{
		ClusterID: clusterName,
		Session:   awsSession,
	}
	token, err := gen.GetWithOptions(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get token with options: %w", err)
	}
	ca, err := base64.StdEncoding.DecodeString(*cluster.CertificateAuthority.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode CA data: %w", err)
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
