package cli

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/nukleros/eks-cluster/pkg/resource"
	kubeerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/dynamic"

	"github.com/threeport/threeport/internal/kube"
	"github.com/threeport/threeport/internal/kubernetesruntime/mapping"
	"github.com/threeport/threeport/internal/provider"
	"github.com/threeport/threeport/internal/threeport"
	"github.com/threeport/threeport/internal/tptdev"
	"github.com/threeport/threeport/internal/util"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	"github.com/threeport/threeport/pkg/auth/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
)

var ThreeportConfigAlreadyExistsErr = errors.New("threeport control plane with provided name already exists in threeport config")

// ControlPlaneCLIArgs is the set of control plane arguments passed to one of
// the CLI tools.
type ControlPlaneCLIArgs struct {
	AuthEnabled             bool
	AwsConfigProfile        string
	AwsConfigEnv            bool
	AwsRegion               string
	CfgFile                 string
	ControlPlaneImageRepo   string
	ControlPlaneImageTag    string
	CreateRootDomain        string
	CreateProviderAccountID string
	CreateAdminEmail        string
	DevEnvironment          bool
	ForceOverwriteConfig    bool
	InstanceName            string
	InfraProvider           string
	KubeconfigPath          string
	NumWorkerNodes          int
	ProviderConfigDir       string
	ThreeportLocalAPIPort   int
	ThreeportPath           string
}

const tier = threeport.ControlPlaneTierDev

// InitArgs sets the default provider config directory, kubeconfig path and path
// to threeport repo as needed in the CLI arguments.
func InitArgs(args *ControlPlaneCLIArgs) {
	// provider config dir
	if args.ProviderConfigDir == "" {
		providerConf, err := config.DefaultProviderConfigDir()
		if err != nil {
			Error("failed to set infra provider config directory", err)
			os.Exit(1)
		}
		args.ProviderConfigDir = providerConf
	}

	// kubeconfig
	defaultKubeconfig, err := kube.DefaultKubeconfig()
	if err != nil {
		Error("failed to get path to default kubeconfig", err)
		os.Exit(1)
	}
	args.KubeconfigPath = defaultKubeconfig

	// set default threeport repo path if not provided
	// this is needed to map the container path to the host path for live
	// reloads of the code
	if args.ThreeportPath == "" {
		tp, err := os.Getwd()
		if err != nil {
			Error("failed to get current working directory", err)
			os.Exit(1)
		}
		args.ThreeportPath = tp
	}
}

// CreateControlPlane uses the CLI arguments to create a new threeport control
// plane.
func (a *ControlPlaneCLIArgs) CreateControlPlane() error {
	// get the threeport config
	threeportConfig, err := config.GetThreeportConfig()
	if err != nil {
		return fmt.Errorf("failed to get threeport config: %w", err)
	}

	// check threeport config for existing instance config
	threeportInstanceConfigExists := threeportConfig.CheckThreeportConfigExists(a.InstanceName)
	if threeportInstanceConfigExists && !a.ForceOverwriteConfig {
		return ThreeportConfigAlreadyExistsErr
	}

	// flag validation
	if err := validateCreateControlPlaneFlags(
		a.InfraProvider,
		a.CreateRootDomain,
		a.CreateProviderAccountID,
		a.AuthEnabled,
	); err != nil {
		return fmt.Errorf("flag validation failed: %w", err)
	}

	// create threeport config for new instance
	threeportInstanceConfig := &config.Instance{
		Name:     a.InstanceName,
		Provider: a.InfraProvider,
	}

	// configure the control plane
	controlPlane := threeport.ControlPlane{
		InfraProvider: threeport.ClusterInfraProvider(a.InfraProvider),
		Tier:          tier,
	}

	// configure the infra provider
	var clusterInfra provider.ClusterInfra
	var threeportAPIEndpoint string
	switch controlPlane.InfraProvider {
	case threeport.ClusterInfraProviderKind:
		threeportAPIEndpoint = threeport.ThreeportLocalAPIEndpoint

		// delete kind cluster if interrupted
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigs
			Warning("received Ctrl+C, removing kind cluster...")
			if err := a.DeleteControlPlane(); err != nil {
				Error("failed to delete kind cluster", err)
			}
			os.Exit(1)
		}()

		// construct kind infra provider object
		clusterInfraKind := provider.ClusterInfraKind{
			ThreeportInstanceName: a.InstanceName,
			KubeconfigPath:        a.KubeconfigPath,
			DevEnvironment:        a.DevEnvironment,
			ThreeportPath:         a.ThreeportPath,
			NumWorkerNodes:        a.NumWorkerNodes,
		}
		// update threerport config
		threeportInstanceConfig.APIServer = fmt.Sprintf(
			"%s:%d",
			threeportAPIEndpoint,
			a.ThreeportLocalAPIPort,
		)

		clusterInfra = &clusterInfraKind
	case threeport.ClusterInfraProviderEKS:
		// create AWS Config
		awsConfig, err := resource.LoadAWSConfig(
			a.AwsConfigEnv,
			a.AwsConfigProfile,
			a.AwsRegion,
		)
		if err != nil {
			return fmt.Errorf("failed to load AWS configuration with local config: %w", err)
		}

		// create a resource client to create EKS resources
		resourceClient := resource.CreateResourceClient(awsConfig)

		// capture messages as resources are created and return to user
		go func() {
			for msg := range *resourceClient.MessageChan {
				Info(msg)
			}
		}()

		// capture inventory and write to file as it is created
		go func() {
			for inventory := range *resourceClient.InventoryChan {
				if err := resource.WriteInventory(
					provider.EKSInventoryFilepath(a.ProviderConfigDir, a.InstanceName),
					&inventory,
				); err != nil {
					Error("failed to write inventory file", err)
				}
			}
		}()

		// delete eks cluster resources if interrupted
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigs
			Warning("received Ctrl+C, cleaning up resources...")
			inventory, err := resource.ReadInventory(
				provider.EKSInventoryFilepath(a.ProviderConfigDir, a.InstanceName),
			)
			if err != nil {
				Error("failed to read eks cluster inventory", err)
			}
			os.Exit(1)
			if err = resourceClient.DeleteResourceStack(inventory); err != nil {
				Error("failed to delete eks cluster resources", err)
			}
			os.Exit(1)
		}()

		// construct eks cluster infra object
		clusterInfraEKS := provider.ClusterInfraEKS{
			ThreeportInstanceName: a.InstanceName,
			AwsAccountID:          a.CreateProviderAccountID,
			AwsConfig:             *awsConfig,
			ResourceClient:        *resourceClient,
		}

		// update threeport config
		threeportInstanceConfig.EKSProviderConfig = config.EKSProviderConfig{
			AwsConfigEnv:     a.AwsConfigEnv,
			AwsConfigProfile: a.AwsConfigProfile,
			AwsRegion:        a.AwsRegion,
			AwsAccountID:     a.CreateProviderAccountID,
		}

		clusterInfra = &clusterInfraEKS
	}

	// create control plane infra
	kubeConnectionInfo, err := clusterInfra.Create()
	if err != nil {
		// since we failed to complete cluster creation, delete it in case a
		// a cluster was created to prevent dangling clusters.
		_ = clusterInfra.Delete()
		return fmt.Errorf("failed to create control plane infra for threeport: %w", err)
	}
	threeportInstanceConfig.KubeAPI = config.KubeAPI{
		APIEndpoint:   kubeConnectionInfo.APIEndpoint,
		CACertificate: util.Base64Encode(kubeConnectionInfo.CACertificate),
		Certificate:   util.Base64Encode(kubeConnectionInfo.Certificate),
		Key:           util.Base64Encode(kubeConnectionInfo.Key),
		EKSToken:      util.Base64Encode(kubeConnectionInfo.EKSToken),
	}

	// create a client and resource mapper to connect to kubernetes cluster
	// API for installing resources
	if err != nil {
		// delete control plane cluster
		if err := clusterInfra.Delete(); err != nil {
			return fmt.Errorf("failed to delete control plane infra, you may have dangling cluster infra resources still running: %w", err)
		}
		return fmt.Errorf("failed to get a Kubernetes client and mapper: %w", err)
	}

	// the cluster instance is the default compute space cluster to be added
	// to the API
	clusterInstName := threeport.BootstrapClusterName(a.InstanceName)
	controlPlaneCluster := true
	defaultCluster := true
	var clusterInstance v0.ClusterInstance
	switch controlPlane.InfraProvider {
	case threeport.ClusterInfraProviderKind:
		clusterInstance = v0.ClusterInstance{
			Instance: v0.Instance{
				Name: &clusterInstName,
			},
			ThreeportControlPlaneCluster: &controlPlaneCluster,
			APIEndpoint:                  &kubeConnectionInfo.APIEndpoint,
			CACertificate:                &kubeConnectionInfo.CACertificate,
			Certificate:                  &kubeConnectionInfo.Certificate,
			Key:                          &kubeConnectionInfo.Key,
			DefaultCluster:               &defaultCluster,
		}
	case threeport.ClusterInfraProviderEKS:
		clusterInstance = v0.ClusterInstance{
			Instance: v0.Instance{
				Name: &clusterInstName,
			},
			ThreeportControlPlaneCluster: &controlPlaneCluster,
			APIEndpoint:                  &kubeConnectionInfo.APIEndpoint,
			CACertificate:                &kubeConnectionInfo.CACertificate,
			ConnectionToken:              &kubeConnectionInfo.EKSToken,
			DefaultCluster:               &defaultCluster,
		}
	}
	dynamicKubeClient, mapper, err := kube.GetClient(&clusterInstance, false)
	if err != nil {
		// delete control plane cluster
		if err := clusterInfra.Delete(); err != nil {
			return fmt.Errorf("failed to delete control plane infra, you may have dangling cluster infra resources still running: %w", err)
		}
		return fmt.Errorf("failed to get a Kubernetes client and mapper: %w", err)
	}

	// install the threeport control plane dependencies
	if err := threeport.InstallThreeportControlPlaneDependencies(
		dynamicKubeClient,
		mapper,
		a.InfraProvider,
	); err != nil {
		// print the error when it happens and then again post-deletion
		Error("failed to install threeport control plane dependencies", err)
		// delete control plane cluster
		if err := clusterInfra.Delete(); err != nil {
			return fmt.Errorf("failed to delete control plane infra, you may have dangling cluster infra resources still running: %w", err)
		}
		return fmt.Errorf("failed to install threeport control plane dependencies: %w", err)
	}

	// if auth is enabled, generate client certificate and add to local config
	var authConfig *auth.AuthConfig
	if a.AuthEnabled {
		// get auth config
		authConfig, err = auth.GetAuthConfig()
		if err != nil {
			return fmt.Errorf("failed to get auth config: %w", err)
		}

		// generate client certificate
		clientCertificate, clientPrivateKey, err := auth.GenerateCertificate(
			authConfig.CAConfig,
			&authConfig.CAPrivateKey,
		)
		if err != nil {
			return fmt.Errorf("failed to generate client certificate and private key: %w", err)
		}

		clientCredentials := &config.Credential{
			Name:       a.InstanceName,
			ClientCert: util.Base64Encode(clientCertificate),
			ClientKey:  util.Base64Encode(clientPrivateKey),
		}

		threeportInstanceConfig.AuthEnabled = true
		threeportInstanceConfig.Credentials = append(threeportInstanceConfig.Credentials, *clientCredentials)
		threeportInstanceConfig.CACert = authConfig.CABase64Encoded

	} else {
		threeportInstanceConfig.AuthEnabled = false
	}

	// update threeport config and refresh threeport config to updated version
	config.UpdateThreeportConfig(threeportConfig, threeportInstanceConfig)
	threeportConfig, err = config.GetThreeportConfig()
	if err != nil {
		return fmt.Errorf("failed to refresh threeport config: %w", err)
	}

	// get threeport API client
	ca, clientCertificate, clientPrivateKey, err := threeportConfig.GetThreeportCertificates()
	if err != nil {
		return fmt.Errorf("failed to get threeport certificates from config: %w", err)
	}
	apiClient, err := client.GetHTTPClient(a.AuthEnabled, ca, clientCertificate, clientPrivateKey)
	if err != nil {
		return fmt.Errorf("failed to create http client: %w", err)
	}

	// for dev environment, build and load dev images for API and controllers
	if a.DevEnvironment {
		if err := tptdev.PrepareDevImages(a.ThreeportPath, provider.ThreeportClusterName(a.InstanceName)); err != nil {
			return fmt.Errorf("failed to build and load dev control plane images: %w", err)
		}
	}

	// install the API
	if err := threeport.InstallThreeportAPI(
		dynamicKubeClient,
		mapper,
		a.DevEnvironment,
		threeportAPIEndpoint,
		a.ControlPlaneImageRepo,
		a.ControlPlaneImageTag,
		authConfig,
		a.InfraProvider,
	); err != nil {
		return fmt.Errorf("failed to install threeport API server: %w", err)
	}

	// for a cloud provider installed control plane, determine the threeport
	// API's remote endpoint to add to the threeport config and to add to the
	// server certificate's alt names when TLS assets are installed
	if a.InfraProvider == "eks" {
		tpapiEndpoint, err := threeport.GetThreeportAPIEndpoint(dynamicKubeClient, *mapper)
		if err != nil {
			// print the error when it happens and then again post-deletion
			Error("failed to get threeport API's public endpoint: %w", err)
			if err := clusterInfra.Delete(); err != nil {
				return fmt.Errorf("failed to delete control plane infra, you may have dangling cluster infra resources still running: %w", err)
			}
			return fmt.Errorf("failed to get threeport API's public endpoint: %w", err)
		}
		threeportAPIEndpoint = tpapiEndpoint
		threeportInstanceConfig.APIServer = fmt.Sprintf("%s:443", threeportAPIEndpoint)
	}

	// if auth enabled install the threeport API TLS assets that include the alt
	// name for the remote load balancer if applicable
	if a.AuthEnabled {
		// install the threeport API TLS assets
		if err := threeport.InstallThreeportAPITLS(
			dynamicKubeClient,
			mapper,
			authConfig,
			threeportAPIEndpoint,
		); err != nil {
			// print the error when it happens and then again post-deletion
			Error("failed to install threeport API TLS assets", err)
			// delete control plane cluster
			if err := clusterInfra.Delete(); err != nil {
				return fmt.Errorf("failed to delete control plane infra: %w", err)
			}
			return fmt.Errorf("failed to install threeport API TLS assets: %w", err)
		}
	}

	// wait for API server to start running - it is not strictly necessary to
	// wait for the API before installing the rest of the control plane, however
	// it is helpful for dev environments and harmless otherwise since the
	// controllers need the API to be running in order to start
	Info("Waiting for threeport API to start running")
	if err := threeport.WaitForThreeportAPI(
		apiClient,
		fmt.Sprintf("%s:%d", threeportAPIEndpoint, a.ThreeportLocalAPIPort),
	); err != nil {
		// print the error when it happens and then again post-deletion
		Error("threeport API did not come up", err)
		// delete control plane cluster
		if err := clusterInfra.Delete(); err != nil {
			return fmt.Errorf("failed to delete control plane infra, you may have dangling cluster infra resources still running: %w", err)
		}
		return fmt.Errorf("threeport API did not come up: %w", err)
	}

	// install the controllers
	if err := threeport.InstallThreeportControllers(
		dynamicKubeClient,
		mapper,
		a.DevEnvironment,
		a.ControlPlaneImageRepo,
		a.ControlPlaneImageTag,
		authConfig,
	); err != nil {
		return fmt.Errorf("failed to install threeport controllers: %w", err)
	}

	// install the agent
	if err := threeport.InstallThreeportAgent(
		dynamicKubeClient,
		mapper,
		a.InstanceName,
		a.DevEnvironment,
		a.ControlPlaneImageRepo,
		a.ControlPlaneImageTag,
		authConfig,
	); err != nil {
		return fmt.Errorf("failed to install threeport agent: %w", err)
	}

	// install support services CRDs
	err = threeport.InstallThreeportCRDs(dynamicKubeClient, mapper)
	if err != nil {
		return fmt.Errorf("failed to install threeport support services CRDs: %w", err)
	}

	// install the support services operator
	err = threeport.InstallThreeportSupportServicesOperator(dynamicKubeClient, mapper, args.DevEnvironment, args.CreateAdminEmail)
	if err != nil {
		return fmt.Errorf("failed to install threeport support services operator: %w", err)
	}

	// update threeport config and refresh threeport config to updated version
	config.UpdateThreeportConfig(threeportConfig, threeportInstanceConfig)
	if err != nil {
		return fmt.Errorf("failed to refresh threeport config: %w", err)
	}

	// create the default compute space cluster definition in threeport API
	clusterDefName := fmt.Sprintf("compute-space-%s", a.InstanceName)
	clusterDefinition := v0.ClusterDefinition{
		Definition: v0.Definition{
			Name: &clusterDefName,
		},
	}
	clusterDefResult, err := client.CreateClusterDefinition(
		apiClient,
		fmt.Sprintf("%s:%d", threeportAPIEndpoint, a.ThreeportLocalAPIPort),
		&clusterDefinition,
	)
	if err != nil {
		// print the error when it happens and then again post-deletion
		Error("failed to create new cluster definition for default compute space", err)
		// delete control plane cluster
		if err := clusterInfra.Delete(); err != nil {
			return fmt.Errorf("failed to delete control plane infra, you may have dangling cluster infra resources still running: %w", err)
		}
		return fmt.Errorf("failed to create new cluster definition for default compute space: %w", err)
	}

	// create default compute space cluster instance in threeport API
	clusterInstance.ClusterDefinitionID = clusterDefResult.ID
	_, err = client.CreateClusterInstance(
		apiClient,
		fmt.Sprintf("%s:%d", threeportAPIEndpoint, a.ThreeportLocalAPIPort),
		&clusterInstance,
	)
	if err != nil {
		// print the error when it happens and then again post-deletion
		Error("failed to create new cluster instance for default compute space", err)
		// delete control plane cluster
		if err := clusterInfra.Delete(); err != nil {
			return fmt.Errorf("failed to delete control plane infra, you may have dangling cluster infra resources still running: %w", err)
		}
		return fmt.Errorf("failed to create new cluster instance for default compute space: %w", err)
	}

	Info("Threeport config updated")

	Complete(fmt.Sprintf("Threeport instance %s created", a.InstanceName))

	return nil
}

// DeleteControlPlane deletes a threeport control plane.
func (a *ControlPlaneCLIArgs) DeleteControlPlane() error {
	// get threeport config
	threeportConfig, err := config.GetThreeportConfig()
	if err != nil {
		return fmt.Errorf("failed to get threeport config: %w", err)
	}

	// check threeport config for existing instance
	// find the threeport instance by name
	threeportInstanceConfigExists := false
	var threeportInstanceConfig config.Instance
	for _, instance := range threeportConfig.Instances {
		if instance.Name == a.InstanceName {
			threeportInstanceConfig = instance
			threeportInstanceConfigExists = true
		}
	}
	if !threeportInstanceConfigExists {
		return errors.New(fmt.Sprintf(
			"config for threeport instance with name %s not found", a.InstanceName,
		))
	}

	var clusterInfra provider.ClusterInfra
	switch threeportInstanceConfig.Provider {
	case threeport.ClusterInfraProviderKind:
		clusterInfraKind := provider.ClusterInfraKind{
			ThreeportInstanceName: threeportInstanceConfig.Name,
			KubeconfigPath:        a.KubeconfigPath,
		}
		clusterInfra = &clusterInfraKind
	case threeport.ClusterInfraProviderEKS:
		// create AWS Config
		awsConfig, err := resource.LoadAWSConfig(
			a.AwsConfigEnv,
			a.AwsConfigProfile,
			a.AwsRegion,
		)
		if err != nil {
			return fmt.Errorf("failed to load AWS configuration with local config: %w", err)
		}

		// create a resource client to create EKS resources
		resourceClient := resource.CreateResourceClient(awsConfig)

		// capture messages as resources are created and return to user
		go func() {
			for msg := range *resourceClient.MessageChan {
				Info(msg)
			}
		}()

		// capture inventory and write to file as it is updated
		go func() {
			for inventory := range *resourceClient.InventoryChan {
				if err := resource.WriteInventory(
					provider.EKSInventoryFilepath(a.ProviderConfigDir, a.InstanceName),
					&inventory,
				); err != nil {
					Error("failed to write inventory file", err)
				}
			}
		}()

		// read inventory to delete
		inventory, err := resource.ReadInventory(provider.EKSInventoryFilepath(a.ProviderConfigDir, a.InstanceName))
		if err != nil {
			return fmt.Errorf("failed to read inventory file for deleting eks cluster resources: %w", err)
		}

		// construct eks cluster infra object
		clusterInfraEKS := provider.ClusterInfraEKS{
			ThreeportInstanceName: threeportInstanceConfig.Name,
			AwsAccountID:          a.CreateProviderAccountID,
			AwsConfig:             *awsConfig,
			ResourceClient:        *resourceClient,
			ResourceInventory:     *inventory,
		}
		clusterInfra = &clusterInfraEKS
	}

	// if provider is EKS we need to delete the threeport API service to
	// check for existing workload instances that may prevent deletion and
	// remove the AWS load balancer before deleting the rest of the infra
	if threeportInstanceConfig.Provider == threeport.ClusterInfraProviderEKS {
		ca, clientCertificate, clientPrivateKey, err := threeportConfig.GetThreeportCertificates()
		if err != nil {
			return fmt.Errorf("failed to get threeport certificates from config: %w", err)
		}
		apiClient, err := client.GetHTTPClient(threeportInstanceConfig.AuthEnabled, ca, clientCertificate, clientPrivateKey)
		if err != nil {
			return fmt.Errorf("failed to create http client: %w", err)
		}

		// check for workload instances on non-kind clusters - halt delete if
		// any are present
		workloadInstances, err := client.GetWorkloadInstances(
			apiClient,
			threeportInstanceConfig.APIServer,
		)
		if err != nil {
			return fmt.Errorf("failed to retrieve workload instances from threeport API: %w", err)
		}
		if len(*workloadInstances) > 0 {
			return errors.New("found workload instances that could prevent control plane deletion - delete all workload instances before deleting control plane")
		}

		// get the cluster instance object
		clusterInstance, err := client.GetThreeportControlPlaneClusterInstance(
			apiClient,
			threeportInstanceConfig.APIServer,
		)
		if err != nil {
			return fmt.Errorf("failed to retrieve cluster instance from threeport API: %w", err)
		}

		// create a client and resource mapper to connect to kubernetes cluster
		// API for deleting resources
		var dynamicKubeClient dynamic.Interface
		var mapper *meta.RESTMapper
		dynamicKubeClient, mapper, err = kube.GetClient(clusterInstance, false)
		if err != nil {
			if kubeerrors.IsUnauthorized(err) {
				// refresh token, save to cluster instance and get kube client
				kubeConn, err := clusterInfra.(*provider.ClusterInfraEKS).RefreshConnection()
				if err != nil {
					return fmt.Errorf("failed to refresh token to connect to EKS cluster: %w", err)
				}
				clusterInstance.ConnectionToken = &kubeConn.EKSToken
				updatedClusterInst, err := client.UpdateClusterInstance(
					apiClient,
					threeportInstanceConfig.APIServer,
					clusterInstance,
				)
				if err != nil {
					return fmt.Errorf("failed to update EKS token on cluster instance: %w", err)
				}
				dynamicKubeClient, mapper, err = kube.GetClient(updatedClusterInst, false)
				if err != nil {
					return fmt.Errorf("failed to get a Kubernetes client and mapper with refreshed token: %w", err)
				}
			} else {
				return fmt.Errorf("failed to get a Kubernetes client and mapper: %w", err)
			}
		}

		// delete threeport API service to remove load balancer
		if err := threeport.UnInstallThreeportControlPlaneComponents(dynamicKubeClient, mapper); err != nil {
			return fmt.Errorf("failed to delete threeport API service: %w", err)
		}
	}

	// delete control plane infra
	if err := clusterInfra.Delete(); err != nil {
		return fmt.Errorf("failed to delete control plane infra: %w", err)
	}

	// update threeport config to remove deleted threeport instance
	config.DeleteThreeportConfigInstance(threeportConfig, a.InstanceName)
	Info("Threeport config updated")

	Complete(fmt.Sprintf("Threeport instance %s deleted", a.InstanceName))

	return nil
}

// validateCreateControlPlaneFlags validates flag inputs as needed
func validateCreateControlPlaneFlags(infraProvider, createRootDomain, createProviderAccountID string, authEnabled bool) error {
	// validate infra provider is supported
	allowedInfraProviders := threeport.SupportedInfraProviders()
	matched := false
	for _, prov := range allowedInfraProviders {
		if threeport.ClusterInfraProvider(infraProvider) == prov {
			matched = true
			break
		}
	}
	if !matched {
		return errors.New(
			fmt.Sprintf(
				"invalid provider value '%s' - must be one of %s",
				infraProvider, allowedInfraProviders,
			),
		)
	}

	// ensure client cert auth is used on remote installations
	if infraProvider != threeport.ClusterInfraProviderKind && !authEnabled {
		return errors.New(
			"cannot turn off client certificate authentication unless using the kind provider",
		)
	}

	// ensure that AWS account ID is provided if using EKS provider
	if infraProvider == threeport.ClusterInfraProviderEKS && createProviderAccountID == "" {
		return errors.New(
			"your AWS account ID must be provided if deploying using the eks provider",
		)
	}

	return nil
}
