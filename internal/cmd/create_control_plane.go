package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/threeport/threeport/internal/cli"
	"github.com/threeport/threeport/internal/kube"
	"github.com/threeport/threeport/internal/provider"
	"github.com/threeport/threeport/internal/threeport"
	"github.com/threeport/threeport/internal/tptdev"
	"github.com/threeport/threeport/internal/util"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	"github.com/threeport/threeport/pkg/auth/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
)

const tier = threeport.ControlPlaneTierDev

func CreateControlPlane(args *cli.ControlPlaneCLIArgs) error {

	// get kubeconfig to use for kind cluster
	if args.KubeconfigPath == "" {
		k, err := kube.DefaultKubeconfig()
		if err != nil {
			return fmt.Errorf("failed to get default kubeconfig path: %w", err)
		}
		args.KubeconfigPath = k
	}

	// set default threeport repo path if not provided
	// this is needed to map the container path to the host path for live
	// reloads of the code
	if args.ThreeportPath == "" {
		tp, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current working directory: %w", err)
		}
		args.ThreeportPath = tp
	}

	// get the threeport config
	threeportConfig, err := config.GetThreeportConfig()
	if err != nil {
		return fmt.Errorf("failed to get threeport config: %w", err)
	}

	// check threeport config for existing instance
	threeportInstanceConfigExists, err := threeportConfig.CheckThreeportConfigExists(
		args.InstanceName,
		args.ForceOverwriteConfig,
	)
	if err != nil {
		return fmt.Errorf("interrupted creation of threeport instance: %w", err)
	}

	// flag validation
	if err := validateCreateControlPlaneFlags(
		args.InfraProvider,
		args.CreateRootDomain,
		args.CreateProviderAccountID,
		args.AuthEnabled,
	); err != nil {
		return fmt.Errorf("flag validation failed: %w", err)
	}

	// create threeport config for new instance
	newThreeportInstance := &config.Instance{
		Name:     args.InstanceName,
		Provider: args.InfraProvider,
	}

	// configure the control plane
	controlPlane := threeport.ControlPlane{
		InfraProvider: threeport.ControlPlaneInfraProvider(args.InfraProvider),
		Tier:          tier,
	}

	// configure the infra provider
	var controlPlaneInfra provider.ControlPlaneInfra
	var threeportAPIEndpoint string
	switch controlPlane.InfraProvider {
	case threeport.ControlPlaneInfraProviderKind:
		threeportAPIEndpoint = threeport.ThreeportLocalAPIEndpoint
		controlPlaneInfraKind := provider.ControlPlaneInfraKind{
			ThreeportInstanceName: args.InstanceName,
			KubeconfigPath:        args.KubeconfigPath,
			ThreeportPath:         args.ThreeportPath,
		}
		kindConfig := controlPlaneInfraKind.GetKindConfig(args.DevEnvironment, args.NumWorkerNodes)
		controlPlaneInfraKind.KindConfig = kindConfig
		controlPlaneInfra = &controlPlaneInfraKind
		newThreeportInstance.APIServer = fmt.Sprintf("%s:%d", threeportAPIEndpoint, args.ThreeportLocalAPIPort)
	case threeport.ControlPlaneInfraProviderEKS:
		controlPlaneInfraEKS := provider.ControlPlaneInfraEKS{
			ThreeportInstanceName: args.InstanceName,
			AwsConfigEnv:          args.AwsConfigEnv,
			AwsConfigProfile:      args.AwsConfigProfile,
			AwsRegion:             args.AwsRegion,
			AwsAccountID:          args.CreateProviderAccountID,
		}
		newThreeportInstance.EKSProviderConfig = config.EKSProviderConfig{
			AwsConfigEnv:     args.AwsConfigEnv,
			AwsConfigProfile: args.AwsConfigProfile,
			AwsRegion:        args.AwsRegion,
			AwsAccountID:     args.CreateProviderAccountID,
		}
		controlPlaneInfra = &controlPlaneInfraEKS
	}

	// create a channel to receive interrupt signals in case user hits
	// Ctrl+C while running
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// create control plane infra
	kubeConnectionInfo, err := controlPlaneInfra.Create(args.ProviderConfigDir, sigs)
	if err != nil {
		// since we failed to complete cluster creation, delete it in case a
		// a cluster was created to prevent dangling clusters.
		_ = controlPlaneInfra.Delete(args.ProviderConfigDir)
		return fmt.Errorf("failed to create control plane infra for threeport: %w", err)
	}
	newThreeportInstance.KubeAPI = config.KubeAPI{
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
		if err := controlPlaneInfra.Delete(args.ProviderConfigDir); err != nil {
			return fmt.Errorf("failed to delete control plane infra, you may have dangling cluster infra resources still running: %w", err)
		}
		return fmt.Errorf("failed to get a Kubernetes client and mapper: %w", err)
	}

	// the cluster instance is the default compute space cluster to be added
	// to the API
	clusterInstName := threeport.BootstrapClusterName(args.InstanceName)
	controlPlaneCluster := true
	defaultCluster := true
	var clusterInstance v0.ClusterInstance
	switch controlPlane.InfraProvider {
	case threeport.ControlPlaneInfraProviderKind:
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
	case threeport.ControlPlaneInfraProviderEKS:
		clusterInstance = v0.ClusterInstance{
			Instance: v0.Instance{
				Name: &clusterInstName,
			},
			ThreeportControlPlaneCluster: &controlPlaneCluster,
			APIEndpoint:                  &kubeConnectionInfo.APIEndpoint,
			CACertificate:                &kubeConnectionInfo.CACertificate,
			ConnectionToken:              &kubeConnectionInfo.EKSToken,
			//AwsConfigEnv:                 &args.AwsConfigEnv,
			//AwsConfigProfile:             &args.AwsConfigProfile,
			//AwsRegion:                    &args.AwsRegion,
			DefaultCluster: &defaultCluster,
		}
	}
	dynamicKubeClient, mapper, err := kube.GetClient(&clusterInstance, false)
	if err != nil {
		// delete control plane cluster
		if err := controlPlaneInfra.Delete(args.ProviderConfigDir); err != nil {
			return fmt.Errorf("failed to delete control plane infra, you may have dangling cluster infra resources still running: %w", err)
		}
		return fmt.Errorf("failed to get a Kubernetes client and mapper: %w", err)
	}

	// install the threeport control plane dependencies
	if err := threeport.InstallThreeportControlPlaneDependencies(
		dynamicKubeClient,
		mapper,
		args.InfraProvider,
	); err != nil {
		// print the error when it happens and then again post-deletion
		cli.Error("failed to install threeport control plane dependencies", err)
		// delete control plane cluster
		if err := controlPlaneInfra.Delete(args.ProviderConfigDir); err != nil {
			return fmt.Errorf("failed to delete control plane infra, you may have dangling cluster infra resources still running: %w", err)
		}
		return fmt.Errorf("failed to install threeport control plane dependencies: %w", err)
	}

	// if auth is enabled, generate client certificate and add to local config
	var authConfig *auth.AuthConfig
	if args.AuthEnabled {
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
			Name:       args.InstanceName,
			ClientCert: util.Base64Encode(clientCertificate),
			ClientKey:  util.Base64Encode(clientPrivateKey),
		}

		newThreeportInstance.AuthEnabled = true
		newThreeportInstance.Credentials = append(newThreeportInstance.Credentials, *clientCredentials)
		newThreeportInstance.CACert = authConfig.CABase64Encoded

		// install the threeport API TLS assets
		if err := threeport.InstallThreeportAPITLS(
			dynamicKubeClient,
			mapper,
			authConfig,
			threeportAPIEndpoint,
		); err != nil {
			// print the error when it happens and then again post-deletion
			cli.Error("failed to install threeport API TLS assets", err)
			// delete control plane cluster
			if err := controlPlaneInfra.Delete(args.ProviderConfigDir); err != nil {
				return fmt.Errorf("failed to delete control plane infra: %w", err)
			}
			return fmt.Errorf("failed to install threeport API TLS assets: %w", err)
		}

	} else {
		newThreeportInstance.AuthEnabled = false
	}

	// update threeport config and refresh threeport config to updated version
	config.UpdateThreeportConfig(threeportInstanceConfigExists, threeportConfig, args.InstanceName, newThreeportInstance)
	threeportConfig, err = config.GetThreeportConfig()
	if err != nil {
		return fmt.Errorf("failed to refresh threeport config: %w", err)
	}

	// get threeport API client
	ca, clientCertificate, clientPrivateKey, err := threeportConfig.GetThreeportCertificates()
	if err != nil {
		return fmt.Errorf("failed to get threeport certificates from config: %w", err)
	}
	apiClient, err := client.GetHTTPClient(args.AuthEnabled, ca, clientCertificate, clientPrivateKey)
	if err != nil {
		return fmt.Errorf("failed to create http client: %w", err)
	}

	if !args.DevEnvironment {
		// install the API
		if err := threeport.InstallThreeportAPI(
			dynamicKubeClient,
			mapper,
			args.DevEnvironment,
			threeportAPIEndpoint,
			args.ControlPlaneImageRepo,
			args.ControlPlaneImageTag,
			authConfig,
			args.InfraProvider,
		); err != nil {
			return fmt.Errorf("failed to install threeport API server: %w", err)
		}

		// install the controllers
		if err := threeport.InstallThreeportControllers(
			dynamicKubeClient,
			mapper,
			args.DevEnvironment,
			args.ControlPlaneImageRepo,
			args.ControlPlaneImageTag,
			authConfig,
		); err != nil {
			return fmt.Errorf("failed to install threeport controllers: %w", err)
		}

		// install the agent
		if err := threeport.InstallThreeportAgent(
			dynamicKubeClient,
			mapper,
			args,
			authConfig,
		); err != nil {
			return fmt.Errorf("failed to install threeport agent: %w", err)
		}

		return nil

	} else {

		// build and load dev images for API and controllers
		if err := tptdev.PrepareDevImages(args.ThreeportPath, provider.ThreeportClusterName(args.InstanceName)); err != nil {
			return fmt.Errorf("failed to build and load dev control plane images: %w", err)
		}

		// install the threeport control plane API and controllers
		if err := threeport.InstallThreeportAPI(
			dynamicKubeClient,
			mapper,
			true,
			threeport.ThreeportLocalAPIEndpoint,
			"",
			"",
			authConfig,
			threeport.ControlPlaneInfraProviderKind,
		); err != nil {
			return fmt.Errorf("failed to install threeport API: %w", err)
		}

		// wait for API server to start running
		cli.Info("waiting for threeport API to start running")
		if err := threeport.WaitForThreeportAPI(
			apiClient, fmt.Sprintf("%s:%d", threeport.ThreeportLocalAPIEndpoint, args.ThreeportLocalAPIPort),
		); err != nil {
			return fmt.Errorf("threeport API did not come up: %w", err)
		}

		// install the threeport controllers - these need to be installed once
		// API server is running in dev environment because the air entrypoint
		// prevents the controllers from crashlooping if they come up before
		// the API server
		if err := threeport.InstallThreeportControllers(
			dynamicKubeClient,
			mapper,
			true,
			"",
			"",
			authConfig,
		); err != nil {
			return fmt.Errorf("failed to install threeport controllers: %w", err)
		}

		// install the agent
		if err := threeport.InstallThreeportAgent(
			dynamicKubeClient,
			mapper,
			args,
			authConfig,
		); err != nil {
			return fmt.Errorf("failed to install threeport agent: %w", err)
		}
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

	//  the threeport API's endpoint
	if args.InfraProvider == "eks" {
		tpapiEndpoint, err := threeport.GetThreeportAPIEndpoint(dynamicKubeClient, *mapper)
		if err != nil {
			// print the error when it happens and then again post-deletion
			cli.Error("failed to get threeport API's public endpoint: %w", err)
			if err := controlPlaneInfra.Delete(args.ProviderConfigDir); err != nil {
				return fmt.Errorf("failed to delete control plane infra, you may have dangling cluster infra resources still running: %w", err)
			}
			return fmt.Errorf("failed to get threeport API's public endpoint: %w", err)
		}
		threeportAPIEndpoint = tpapiEndpoint
		newThreeportInstance.APIServer = fmt.Sprintf("%s:443", threeportAPIEndpoint)
	}

	// update threeport config and refresh threeport config to updated version
	config.UpdateThreeportConfig(threeportInstanceConfigExists, threeportConfig, args.InstanceName, newThreeportInstance)
	if err != nil {
		return fmt.Errorf("failed to refresh threeport config: %w", err)
	}

	// wait for API server to start running
	cli.Info("waiting for threeport API to start running")
	if err := threeport.WaitForThreeportAPI(
		apiClient,
		fmt.Sprintf("%s:%d", threeportAPIEndpoint, args.ThreeportLocalAPIPort),
	); err != nil {
		// print the error when it happens and then again post-deletion
		cli.Error("threeport API did not come up", err)
		// delete control plane cluster
		if err := controlPlaneInfra.Delete(args.ProviderConfigDir); err != nil {
			return fmt.Errorf("failed to delete control plane infra, you may have dangling cluster infra resources still running: %w", err)
		}
		return fmt.Errorf("threeport API did not come up: %w", err)
	}

	// create the default compute space cluster definition in threeport API
	clusterDefName := fmt.Sprintf("compute-space-%s", args.InstanceName)
	clusterDefinition := v0.ClusterDefinition{
		Definition: v0.Definition{
			Name: &clusterDefName,
		},
	}
	clusterDefResult, err := client.CreateClusterDefinition(
		apiClient,
		fmt.Sprintf("%s:%d", threeportAPIEndpoint, args.ThreeportLocalAPIPort),
		&clusterDefinition,
	)
	if err != nil {
		// print the error when it happens and then again post-deletion
		cli.Error("failed to create new cluster definition for default compute space", err)
		// delete control plane cluster
		if err := controlPlaneInfra.Delete(args.ProviderConfigDir); err != nil {
			return fmt.Errorf("failed to delete control plane infra, you may have dangling cluster infra resources still running: %w", err)
		}
		return fmt.Errorf("failed to create new cluster definition for default compute space: %w", err)
	}

	// create default compute space cluster instance in threeport API
	clusterInstance.ClusterDefinitionID = clusterDefResult.ID
	_, err = client.CreateClusterInstance(
		apiClient,
		fmt.Sprintf("%s:%d", threeportAPIEndpoint, args.ThreeportLocalAPIPort),
		&clusterInstance,
	)
	if err != nil {
		// print the error when it happens and then again post-deletion
		cli.Error("failed to create new cluster instance for default compute space", err)
		// delete control plane cluster
		if err := controlPlaneInfra.Delete(args.ProviderConfigDir); err != nil {
			return fmt.Errorf("failed to delete control plane infra, you may have dangling cluster infra resources still running: %w", err)
		}
		return fmt.Errorf("failed to create new cluster instance for default compute space: %w", err)
	}

	cli.Info("threeport config updated")

	cli.Complete(fmt.Sprintf("threeport instance %s created", args.InstanceName))
	return nil

}

// validateCreateControlPlaneFlags validates flag inputs as needed
func validateCreateControlPlaneFlags(infraProvider, createRootDomain, createProviderAccountID string, authEnabled bool) error {
	// validate infra provider is supported
	allowedInfraProviders := threeport.SupportedInfraProviders()
	matched := false
	for _, prov := range allowedInfraProviders {
		if threeport.ControlPlaneInfraProvider(infraProvider) == prov {
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
	if infraProvider != threeport.ControlPlaneInfraProviderKind && !authEnabled {
		return errors.New(
			"cannot turn off client certificate authentication unless using the kind provider",
		)
	}

	// ensure that AWS account ID is provided if using EKS provider
	if infraProvider == threeport.ControlPlaneInfraProviderEKS && createProviderAccountID == "" {
		return errors.New(
			"your AWS account ID must be provided if deploying using the eks provider",
		)
	}

	return nil
}
