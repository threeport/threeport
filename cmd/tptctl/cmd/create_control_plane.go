/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/threeport/threeport/internal/cli"
	"github.com/threeport/threeport/internal/kube"
	"github.com/threeport/threeport/internal/provider"
	"github.com/threeport/threeport/internal/threeport"
	"github.com/threeport/threeport/internal/util"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	"github.com/threeport/threeport/pkg/auth/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
)

// TODO: will become a variable once production-ready control plane instances are
// available.
const tier = threeport.ControlPlaneTierDev

var (
	createThreeportInstanceName string
	createRootDomain            string
	createProviderAccountID     string
	createAdminEmail            string
	forceOverwriteConfig        bool
	authEnabled                 bool
	infraProvider               string
	kindKubeconfigPath          string
	controlPlaneImageRepo       string
	controlPlaneImageTag        string
	threeportLocalAPIPort       int
	numWorkerNodes              int
	awsConfigProfile            string
	awsConfigEnv                bool
	awsRegion                   string
)

// CreateControlPlaneCmd represents the create threeport command
var CreateControlPlaneCmd = &cobra.Command{
	Use:          "control-plane",
	Example:      "tptctl create control-plane --name my-threeport",
	Short:        "Create a new instance of the Threeport control plane",
	Long:         `Create a new instance of the Threeport control plane.`,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		// get the threeport config
		threeportConfig, err := config.GetThreeportConfig()
		if err != nil {
			cli.Error("failed to get threeport config", err)
		}

		// check threeport config for exisiting instance
		threeportInstanceConfigExists, err := threeportConfig.CheckThreeportConfigExists(
			createThreeportInstanceName,
			forceOverwriteConfig,
		)
		if err != nil {
			cli.Error("interupted creation of threeport instance", err)
			cli.Info("if you wish to overwrite the existing config use --force-overwrite-config flag")
			cli.Warning("you will lose the ability to connect to the existing threeport instance if it still exists")
			os.Exit(1)
		}

		// flag validation
		if err := validateCreateControlPlaneFlags(
			infraProvider,
			createRootDomain,
			createProviderAccountID,
		); err != nil {
			cli.Error("flag validation failed", err)
			os.Exit(1)
		}

		// create threeport config for new instance
		newThreeportInstance := &config.Instance{
			Name:     createThreeportInstanceName,
			Provider: infraProvider,
		}

		// configure the control plane
		controlPlane := threeport.ControlPlane{
			InfraProvider: threeport.ControlPlaneInfraProvider(infraProvider),
			Tier:          tier,
		}

		// configure the infra provider
		var controlPlaneInfra provider.ControlPlaneInfra
		var threeportAPIEndpoint string
		switch controlPlane.InfraProvider {
		case threeport.ControlPlaneInfraProviderKind:
			threeportAPIEndpoint = threeport.ThreeportLocalAPIEndpoint
			// get kubeconfig to use for kind cluster
			if kindKubeconfigPath == "" {
				k, err := kube.DefaultKubeconfig()
				if err != nil {
					cli.Error("failed to get default kubeconfig path", err)
					os.Exit(1)
				}
				kindKubeconfigPath = k
			}
			controlPlaneInfraKind := provider.ControlPlaneInfraKind{
				ThreeportInstanceName: createThreeportInstanceName,
				KubeconfigPath:        kindKubeconfigPath,
			}
			devEnvironment := false
			kindConfig := controlPlaneInfraKind.GetKindConfig(devEnvironment, numWorkerNodes)
			controlPlaneInfraKind.KindConfig = kindConfig
			controlPlaneInfra = &controlPlaneInfraKind
			newThreeportInstance.APIServer = fmt.Sprintf("%s:%d", threeportAPIEndpoint, threeportLocalAPIPort)
		case threeport.ControlPlaneInfraProviderEKS:
			controlPlaneInfraEKS := provider.ControlPlaneInfraEKS{
				ThreeportInstanceName: createThreeportInstanceName,
				AWSConfigEnv:          awsConfigEnv,
				AWSConfigProfile:      awsConfigProfile,
				AWSRegion:             awsRegion,
				AWSAccountID:          createProviderAccountID,
			}
			newThreeportInstance.EKSProviderConfig = config.EKSProviderConfig{
				AWSConfigEnv:     awsConfigEnv,
				AWSConfigProfile: awsConfigProfile,
				AWSRegion:        awsRegion,
				AWSAccountID:     createProviderAccountID,
			}
			controlPlaneInfra = &controlPlaneInfraEKS
		}

		// if auth is enabled, generate client certificate and add to local config
		var authConfig *auth.AuthConfig
		if authEnabled {
			authConfig, err = auth.GetAuthConfig()
			if err != nil {
				cli.Error("failed to get auth config", err)
				os.Exit(1)
			}

			// generate client certificate
			clientCertificate, clientPrivateKey, err := auth.GenerateCertificate(
				authConfig.CAConfig,
				&authConfig.CAPrivateKey,
			)
			if err != nil {
				cli.Error("failed to generate client certificate and private key", err)
				os.Exit(1)
			}

			clientCredentials := &config.Credential{
				Name:       createThreeportInstanceName,
				ClientCert: util.Base64Encode(clientCertificate),
				ClientKey:  util.Base64Encode(clientPrivateKey),
			}

			newThreeportInstance.AuthEnabled = true
			newThreeportInstance.Credentials = append(newThreeportInstance.Credentials, *clientCredentials)
			newThreeportInstance.CACert = authConfig.CABase64Encoded
		} else {
			newThreeportInstance.AuthEnabled = false
		}

		// create control plane infra
		kubeConnectionInfo, err := controlPlaneInfra.Create(providerConfigDir)
		if err != nil {
			// since we failed to complete cluster creation, delete it in case a
			// a cluster was created to prevent dangling clusters.
			_ = controlPlaneInfra.Delete(providerConfigDir)
			cli.Error("failed to get create control plane infra for threeport", err)
			os.Exit(1)
		}
		newThreeportInstance.KubeAPI = config.KubeAPI{
			APIEndpoint:   kubeConnectionInfo.APIEndpoint,
			CACertificate: util.Base64Encode(kubeConnectionInfo.CACertificate),
			Certificate:   util.Base64Encode(kubeConnectionInfo.Certificate),
			Key:           util.Base64Encode(kubeConnectionInfo.Key),
			EKSToken:      util.Base64Encode(kubeConnectionInfo.EKSToken),
		}

		// the cluster instance is the default compute space cluster to be added
		// to the API
		clusterInstName := threeport.BootstrapClusterName(createThreeportInstanceName)
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
				EKSToken:                     &kubeConnectionInfo.EKSToken,
				AWSConfigEnv:                 &awsConfigEnv,
				AWSConfigProfile:             &awsConfigProfile,
				AWSRegion:                    &awsRegion,
				DefaultCluster:               &defaultCluster,
			}
		}

		// create a client and resource mapper to connect to kubernetes cluster
		// API for installing resources
		dynamicKubeClient, mapper, err := kube.GetClient(&clusterInstance, false)
		if err != nil {
			// delete control plane cluster
			if err := controlPlaneInfra.Delete(providerConfigDir); err != nil {
				cli.Error("failed to delete control plane infra", err)
				cli.Warning("you may have dangling cluster infra resources still running")
			}
			cli.Error("failed to get a Kubernetes client and mapper", err)
			os.Exit(1)
		}

		// install the threeport control plane system services
		if err := threeport.InstallThreeportSystemServices(
			dynamicKubeClient,
			mapper,
			provider.ThreeportClusterName(createThreeportInstanceName),
		); err != nil {
			// print the error when it happens and then again post-deletion
			cli.Error("failed to install threeport control plane system services", err)
			// delete control plane cluster
			if err := controlPlaneInfra.Delete(providerConfigDir); err != nil {
				cli.Error("failed to delete control plane infra", err)
				cli.Warning("you may have dangling cluster infra resources still running")
			}
			cli.Error("failed to install threeport control plane system services", err)
			os.Exit(1)
		}

		// install the threeport control plane dependencies
		if err := threeport.InstallThreeportControlPlaneDependencies(
			dynamicKubeClient,
			mapper,
			infraProvider,
		); err != nil {
			// print the error when it happens and then again post-deletion
			cli.Error("failed to install threeport control plane dependencies", err)
			// delete control plane cluster
			if err := controlPlaneInfra.Delete(providerConfigDir); err != nil {
				cli.Error("failed to delete control plane infra", err)
				cli.Warning("you may have dangling cluster infra resources still running")
			}
			cli.Error("failed to install threeport control plane dependencies", err)
			os.Exit(1)
		}

		// install the threeport control plane API and controllers
		if err := threeport.InstallThreeportControlPlaneComponents(
			dynamicKubeClient,
			mapper,
			false,
			threeportAPIEndpoint,
			controlPlaneImageRepo,
			controlPlaneImageTag,
			authConfig,
			infraProvider,
		); err != nil {
			// print the error when it happens and then again post-deletion
			cli.Error("failed to install threeport control plane components", err)
			// delete control plane cluster
			if err := controlPlaneInfra.Delete(providerConfigDir); err != nil {
				cli.Error("failed to delete control plane infra", err)
				cli.Warning("you may have dangling cluster infra resources still running")
			}
			cli.Error("failed to install threeport control plane components", err)
			os.Exit(1)
		}

		// get the threeport API's endpoint
		if infraProvider == "eks" {
			tpapiEndpoint, err := threeport.GetThreeportAPIEndpoint(dynamicKubeClient, *mapper)
			if err != nil {
				// print the error when it happens and then again post-deletion
				cli.Error("failed to get threeport API's public endpoint: %w", err)
				if err := controlPlaneInfra.Delete(providerConfigDir); err != nil {
					cli.Error("failed to delete control plane infra", err)
					cli.Warning("you may have dangling cluster infra resources still running")
				}
				cli.Error("failed to get threeport API's public endpoint: %w", err)
				os.Exit(1)
			}
			threeportAPIEndpoint = tpapiEndpoint
			newThreeportInstance.APIServer = fmt.Sprintf("%s:443", threeportAPIEndpoint)
		}

		// update threeport config and refresh threeport config to updated version
		config.UpdateThreeportConfig(threeportInstanceConfigExists, threeportConfig, createThreeportInstanceName, newThreeportInstance)
		threeportConfig, err = config.GetThreeportConfig()
		if err != nil {
			cli.Error("failed to refresh threeport config", err)
		}

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
			if err := controlPlaneInfra.Delete(providerConfigDir); err != nil {
				cli.Error("failed to delete control plane infra", err)
				cli.Warning("you may have dangling cluster infra resources still running")
			}
			cli.Error("failed to install threeport API TLS assets", err)
			os.Exit(1)
		}

		// get threeport API client
		ca, clientCertificate, clientPrivateKey, err := threeportConfig.GetThreeportCertificates()
		if err != nil {
			cli.Error("failed to get threeport certificates from config", err)
			os.Exit(1)
		}
		apiClient, err := client.GetHTTPClient(authEnabled, ca, clientCertificate, clientPrivateKey)
		if err != nil {
			cli.Error("failed to create http client", err)
			os.Exit(1)
		}

		// wait for API server to start running
		cli.Info("waiting for threeport API to start running")
		if err := threeport.WaitForThreeportAPI(
			apiClient,
			fmt.Sprintf("%s:%d", threeportAPIEndpoint, threeportLocalAPIPort),
		); err != nil {
			// print the error when it happens and then again post-deletion
			cli.Error("threeport API did not come up", err)
			// delete control plane cluster
			if err := controlPlaneInfra.Delete(providerConfigDir); err != nil {
				cli.Error("failed to delete control plane infra", err)
				cli.Warning("you may have dangling cluster infra resources still running")
			}
			cli.Error("threeport API did not come up", err)
			os.Exit(1)
		}

		// create the default compute space cluster definition in threeport API
		clusterDefName := fmt.Sprintf("compute-space-%s", createThreeportInstanceName)
		clusterDefinition := v0.ClusterDefinition{
			Definition: v0.Definition{
				Name: &clusterDefName,
			},
		}
		clusterDefResult, err := client.CreateClusterDefinition(
			apiClient,
			fmt.Sprintf("%s:%d", threeportAPIEndpoint, threeportLocalAPIPort),
			&clusterDefinition,
		)
		if err != nil {
			// print the error when it happens and then again post-deletion
			cli.Error("failed to create new cluster definition for default compute space", err)
			// delete control plane cluster
			if err := controlPlaneInfra.Delete(providerConfigDir); err != nil {
				cli.Error("failed to delete control plane infra", err)
				cli.Warning("you may have dangling cluster infra resources still running")
			}
			cli.Error("failed to create new cluster definition for default compute space", err)
			os.Exit(1)
		}

		// create default compute space cluster instance in threeport API
		clusterInstance.ClusterDefinitionID = clusterDefResult.ID
		_, err = client.CreateClusterInstance(
			apiClient,
			fmt.Sprintf("%s:%d", threeportAPIEndpoint, threeportLocalAPIPort),
			&clusterInstance,
		)
		if err != nil {
			// print the error when it happens and then again post-deletion
			cli.Error("failed to create new cluster instance for default compute space", err)
			// delete control plane cluster
			if err := controlPlaneInfra.Delete(providerConfigDir); err != nil {
				cli.Error("failed to delete control plane infra", err)
				cli.Warning("you may have dangling cluster infra resources still running")
			}
			cli.Error("failed to create new cluster instance for default compute space", err)
			os.Exit(1)
		}

		cli.Info("threeport config updated")

		cli.Complete(fmt.Sprintf("threeport instance %s created", createThreeportInstanceName))
	},
}

func init() {
	createCmd.AddCommand(CreateControlPlaneCmd)
	CreateControlPlaneCmd.Flags().StringVarP(
		&createThreeportInstanceName,
		"name", "n", "", "Required. Name of control plane instance.",
	)
	CreateControlPlaneCmd.MarkFlagRequired("name")
	CreateControlPlaneCmd.Flags().StringVarP(
		&infraProvider,
		"provider", "p", "kind", fmt.Sprintf("The infrasture provider to install upon. Supported infra providers: %s", threeport.SupportedInfraProviders()),
	)
	// this flag will be enabled once production-ready control plane instances
	// are available.
	//CreateControlPlaneCmd.Flags().StringVarP(
	//	&tier,
	//	"tier", "t", threeport.ControlPlaneTierDev, "Determines the level of availability and data retention for the control plane.",
	//)
	CreateControlPlaneCmd.Flags().StringVar(
		&kindKubeconfigPath,
		"kind-kubeconfig", "", "Path to kubeconfig used for kind provider installs (default is ~/.kube/config).",
	)
	CreateControlPlaneCmd.Flags().StringVar(
		&awsConfigProfile,
		"aws-config-profile", "default", "The AWS config profile to draw credentials from when using eks provider.",
	)
	CreateControlPlaneCmd.Flags().BoolVar(
		&awsConfigEnv,
		"aws-config-env", false, "Retrieve AWS credentials from environment variables when using eks provider.",
	)
	CreateControlPlaneCmd.Flags().StringVar(
		&awsRegion,
		"aws-region", "", "AWS region code to install threeport in when using eks provider.",
	)
	CreateControlPlaneCmd.Flags().BoolVar(
		&forceOverwriteConfig,
		"force-overwrite-config", false, "Force the overwrite of an existing Threeport instance config.  Warning: this will erase the connection info for the existing instance.  Only do this if the existing instance has already been deleted and is no longer in use.",
	)
	CreateControlPlaneCmd.Flags().BoolVar(
		&authEnabled,
		"auth-enabled", true, "Enable client certificate authentication. Can only be disabled when using kind provider.",
	)
	CreateControlPlaneCmd.Flags().StringVar(
		&createRootDomain,
		"root-domain", "", "The root domain name to use for the Threeport API. Requires a public hosted zone in AWS Route53. A subdomain for the Threeport API will be added to the root domain.",
	)
	CreateControlPlaneCmd.Flags().StringVar(
		&createProviderAccountID,
		"provider-account-id", "", "The provider account ID.  Required if providing a root domain for automated DNS management.",
	)
	CreateControlPlaneCmd.Flags().StringVar(
		&createAdminEmail,
		"admin-email", "", "Email address of control plane admin.  Provided to TLS provider.",
	)
	CreateControlPlaneCmd.Flags().StringVarP(
		&controlPlaneImageRepo,
		"control-plane-image-repo", "i", "", "Alternate image repo to pull threeport control plane images from.",
	)
	CreateControlPlaneCmd.Flags().StringVarP(
		&controlPlaneImageTag,
		"control-plane-image-tag", "t", "", "Alternate image tag to pull threeport control plane images from.",
	)
	CreateControlPlaneCmd.Flags().IntVar(
		&threeportLocalAPIPort,
		"threeport-api-port", 443, "Local port to bind threeport APIServer to. Only applies to kind provider.")
	CreateControlPlaneCmd.Flags().IntVar(
		&numWorkerNodes,
		"num-worker-nodes", 0, "Number of additional worker nodes to deploy. Only applies to kind provider. (default is 0)")
}

// validateCreateControlPlaneFlags validates flag inputs as needed
func validateCreateControlPlaneFlags(infraProvider, createRootDomain, createProviderAccountID string) error {
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
