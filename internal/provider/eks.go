package provider

import (
	"fmt"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	aws_v1 "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/nukleros/eks-cluster/pkg/api"
	"github.com/nukleros/eks-cluster/pkg/resource"

	"github.com/threeport/threeport/internal/kube"
	"github.com/threeport/threeport/internal/threeport"
)

type ControlPlaneInfraEKS struct {
	ThreeportInstanceName string
	AwsConfigEnv          bool
	AwsConfigProfile      string
	AwsRegion             string
	AwsAccountID          string
	AwsAccessKeyID        string
	AwsSecretAccessKey    string
}

// Create installs a Kubernetes cluster using AWS EKS for the threeport control
// plane.
func (i *ControlPlaneInfraEKS) Create(providerConfigDir string, sigs chan os.Signal) (*kube.KubeConnectionInfo, error) {
	// create an AWS config to connect to AWS API
	var awsConfig aws.Config
	if i.AwsAccessKeyID != "" && i.AwsSecretAccessKey != "" {
		config, err := config.LoadDefaultConfig(
			context.Background(),
			config.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider(
					i.AwsAccessKeyID,
					i.AwsSecretAccessKey,
					"",
				),
			),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to load AWS configuration with static credentials: %w", err)
		}
		awsConfig = config
	} else {
		config, err := resource.LoadAWSConfig(
			i.AwsConfigEnv,
			i.AwsConfigProfile,
			i.AwsRegion,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to load AWS configuration with local config: %w", err)
		}
		awsConfig = *config
	}

	// create a resource client to create EKS resources
	resourceClient, err := api.CreateResourceClient(i.AwsConfigEnv, i.AwsConfigProfile)
	if err != nil {
		return nil, err
	}

	// delete resource stack if user interrupts creation with Ctrl+C
	go func() {
		<-sigs
		cli.Info("\nreceived interrupt signal, cleaning up resources...")
		if err := resourceClient.DeleteResourceStack(
			inventoryFilepath(providerConfigDir, i.ThreeportInstanceName),
		); err != nil {
			cli.Error("\nfailed to delete EKS resources", err)
			os.Exit(1)
		}
		os.Exit(0)
	}()

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
	i.ResourceClient.CreateResourceStack(resourceConfig)

	// get kubernetes API connection info
	kubeConnInfo, err := getEKSConnectionInfo(
		&awsConfig,
		i.AwsConfigProfile,
		ThreeportClusterName(i.ThreeportInstanceName),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to EKS cluster connection info: %w", err)
	}

	return &kubeConnInfo, nil
}

// Delete deletes an AWS EKS cluster and the threeport control plane with it.
func (i *ControlPlaneInfraEKS) Delete(providerConfigDir string) error {

	// create a resource client for spinning up AWS resources and getting status
	// messages back as it progresses
	resourceClient, err := api.CreateResourceClient(i.AwsConfigEnv, i.AwsConfigProfile)
	if err != nil {
		return err
	}

	// delete EKS cluster resources
	if err := i.ResourceClient.DeleteResourceStack(&i.ResourceInventory); err != nil {
		return fmt.Errorf("failed to delete eks cluster resource stack: %w", err)
	}

	return nil
}

// RefreshConnection gets a new token for authentication to an EKS cluster.
func (i *ControlPlaneInfraEKS) RefreshConnection() (*kube.KubeConnectionInfo, error) {
	// create an AWS config to connect to AWS API
	awsConfig, err := resource.LoadAWSConfig(
		i.AwsConfigEnv,
		i.AwsConfigProfile,
		i.AwsRegion,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS configuration: %w", err)
	}

	return getEKSConnectionInfo(
		awsConfig,
		i.AwsConfigProfile,
		ThreeportClusterName(i.ThreeportInstanceName),
	)
}

// getEKSConnectionInfo queries AWS for the connection token and returns the
// connection info for a particular cluster name.
func getEKSConnectionInfo(awsConfig *aws.Config, awsProfile, clusterName string) (*kube.KubeConnectionInfo, error) {
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

	// create a new session using the v1 SDK which is used by
	// sigs.k8s.io/aws-iam-authenticator/pkg/token to get a token
	sessionOpts := session.Options{
		Profile: awsProfile,
		Config: aws_v1.Config{
			Region: aws_v1.String(awsConfig.Region),
		},
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
		APIEndpoint:   eksClusterConn.APIEndpoint,
		CACertificate: eksClusterConn.CACertificate,
		EKSToken:      eksClusterConn.Token,
	}

	return &kubeConnInfo, nil
}

// EKSInventoryFilepath returns a standardized filename and path for the EKS
// inventory file.
func EKSInventoryFilepath(providerConfigDir, instanceName string) string {
	inventoryFilename := fmt.Sprintf("eks-inventory-%s.json", instanceName)
	return filepath.Join(providerConfigDir, inventoryFilename)
}
