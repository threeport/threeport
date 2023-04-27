/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"crypto/x509"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/threeport/threeport/internal/cli"
	"github.com/threeport/threeport/internal/kube"
	"github.com/threeport/threeport/internal/provider"
	"github.com/threeport/threeport/internal/threeport"
	v0 "github.com/threeport/threeport/pkg/api/v0"
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
	infraProvider               string
	kubeconfigPath              string
	controlPlaneImageRepo       string
)

// CreateControlPlaneCmd represents the create threeport command
var CreateControlPlaneCmd = &cobra.Command{
	Use:          "control-plane",
	Example:      "tptctl create control-plane --name my-threeport",
	Short:        "Create a new instance of the Threeport control plane",
	Long:         `Create a new instance of the Threeport control plane.`,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		// get threeport config
		threeportConfig := &config.ThreeportConfig{}
		if err := viper.Unmarshal(threeportConfig); err != nil {
			cli.Error("failed to get threeport config", err)
			os.Exit(1)
		}

		// check threeport config for exisiting instance
		threeportInstanceConfigExists := false
		for _, instance := range threeportConfig.Instances {
			if instance.Name == createThreeportInstanceName {
				threeportInstanceConfigExists = true
				if !forceOverwriteConfig {
					cli.Error(
						"interupted creation of threeport instance",
						errors.New(fmt.Sprintf("instance of threeport with name %s already exists", instance.Name)),
					)
					cli.Info("if you wish to overwrite the existing config use --force-overwrite-config flag")
					cli.Warning("you will lose the ability to connect to the existing threeport instance if it still exists")
					os.Exit(1)
				}
			}
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

		// configure the control plane
		controlPlane := threeport.ControlPlane{
			InfraProvider: threeport.ControlPlaneInfraProvider(infraProvider),
			Tier:          tier,
		}

		// configure the infra provider
		var controlPlaneInfra provider.ControlPlaneInfra
		var threeportAPIEndpoint string
		var threeportAPIProtocol string
		switch controlPlane.InfraProvider {
		case threeport.ControlPlaneInfraProviderKind:
			threeportAPIEndpoint = threeport.ThreeportLocalAPIEndpoint
			threeportAPIProtocol = threeport.ThreeportLocalAPIProtocol
			// get kubeconfig to use for kind cluster
			if kubeconfigPath == "" {
				k, err := kube.DefaultKubeconfig()
				if err != nil {
					cli.Error("failed to get default kubeconfig path", err)
					os.Exit(1)
				}
				kubeconfigPath = k
			}
			controlPlaneInfraKind := provider.ControlPlaneInfraKind{
				ThreeportInstanceName: createThreeportInstanceName,
				KubeconfigPath:        kubeconfigPath,
			}
			devEnvironment := false
			kindConfig := controlPlaneInfraKind.GetKindConfig(devEnvironment)
			controlPlaneInfraKind.KindConfig = kindConfig
			controlPlaneInfra = &controlPlaneInfraKind
		}

		// create control plane
		kubeConnectionInfo, err := controlPlaneInfra.Create()
		if err != nil {
			// since we failed to complete cluster creation, delete it in case a
			// a cluster was created to prevent dangling clusters.
			_ = controlPlaneInfra.Delete()
			cli.Error("failed to get create control plane infra for threeport", err)
			os.Exit(1)
		}

		// the cluster instance is the default compute space cluster to be added
		// to the API
		clusterInstName := fmt.Sprintf("compute-space-%s-0", createThreeportInstanceName)
		controlPlaneCluster := true
		defaultCluster := true
		clusterInstance := v0.ClusterInstance{
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

		// create a client to connect to kind cluster kube API
		dynamicKubeClient, mapper, err := kube.GetClient(&clusterInstance, false)
		if err != nil {
			// delete control plane cluster
			if err := controlPlaneInfra.Delete(); err != nil {
				cli.Error("failed to delete control plane infra", err)
				cli.Warning("you may have a dangling kind cluster still running")
			}
			cli.Error("failed to get a Kubernetes client and mapper", err)
			os.Exit(1)
		}

		// install the threeport control plane dependencies
		if err := threeport.InstallThreeportControlPlaneDependencies(dynamicKubeClient, mapper); err != nil {
			// delete control plane cluster
			if err := controlPlaneInfra.Delete(); err != nil {
				cli.Error("failed to delete control plane infra", err)
				cli.Warning("you may have a dangling kind cluster still running")
			}
			cli.Error("failed to install threeport control plane dependencies", err)
			os.Exit(1)
		}

		// generate certificate authority for the threeport API
		caConfig, ca, caPrivateKey, err := threeport.GenerateCACertificate()
		if err != nil {
			cli.Error("failed to generate certificate authority and private key", err)
			os.Exit(1)
		}

		// generate server certificate
		serverCertificate, serverPrivateKey, err := threeport.GenerateCertificate(caConfig, caPrivateKey)
		if err != nil {
			cli.Error("failed to generate server certificate and private key", err)
			os.Exit(1)
		}

		// get PEM-encoded keypairs as strings to pass into deployment manifests
		caEncoded := threeport.GetPEMEncoding(ca, "CERTIFICATE")
		caPrivateKeyEncoded := threeport.GetPEMEncoding(x509.MarshalPKCS1PrivateKey(caPrivateKey), "RSA PRIVATE KEY")
		serverCertificateEncoded := threeport.GetPEMEncoding(serverCertificate, "CERTIFICATE")
		serverPrivateKeyEncoded := threeport.GetPEMEncoding(x509.MarshalPKCS1PrivateKey(serverPrivateKey), "RSA PRIVATE KEY")

		// install the threeport control plane API and controllers
		if err := threeport.InstallThreeportControlPlaneComponents(
			dynamicKubeClient,
			mapper,
			false,
			threeportAPIEndpoint,
			controlPlaneImageRepo,
			caEncoded,
			caPrivateKeyEncoded,
			serverCertificateEncoded,
			serverPrivateKeyEncoded,
		); err != nil {
			// delete control plane cluster
			if err := controlPlaneInfra.Delete(); err != nil {
				cli.Error("failed to delete control plane infra", err)
				cli.Warning("you may have a dangling kind cluster still running")
			}
			cli.Error("failed to install threeport control plane components", err)
			os.Exit(1)
		}

		httpsClient, err := client.GetHTTPSClient()
		if err != nil {
			fmt.Errorf("failed to create https client: %w", err)
			os.Exit(1)
		}

		// wait for API server to start running
		cli.Info("waiting for threeport API to start running")
		if err := threeport.WaitForThreeportAPI(
			httpsClient,
			fmt.Sprintf("%s://%s", threeportAPIProtocol, threeportAPIEndpoint),
		); err != nil {
			// delete control plane cluster
			if err := controlPlaneInfra.Delete(); err != nil {
				cli.Error("failed to delete control plane infra", err)
				cli.Warning("you may have a dangling kind cluster still running")
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
			httpsClient,
			&clusterDefinition,
			fmt.Sprintf("%s://%s", threeportAPIProtocol, threeportAPIEndpoint),
		)
		if err != nil {
			// delete control plane cluster
			if err := controlPlaneInfra.Delete(); err != nil {
				cli.Error("failed to delete control plane infra", err)
				cli.Warning("you may have a dangling kind cluster still running")
			}
			cli.Error("failed to create new cluster definition for default compute space", err)
			os.Exit(1)
		}

		// create default compute space cluster instance in threeport API
		clusterInstance.ClusterDefinitionID = clusterDefResult.ID
		_, err = client.CreateClusterInstance(
			httpsClient,
			&clusterInstance,
			fmt.Sprintf("%s://%s", threeportAPIProtocol, threeportAPIEndpoint),
		)
		if err != nil {
			// delete control plane cluster
			if err := controlPlaneInfra.Delete(); err != nil {
				cli.Error("failed to delete control plane infra", err)
				cli.Warning("you may have a dangling kind cluster still running")
			}
			cli.Error("failed to create new cluster instance for default compute space", err)
			os.Exit(1)
		}

		// create threeport config for new instance
		newThreeportInstance := &config.Instance{
			Name:       createThreeportInstanceName,
			Provider:   infraProvider,
			APIServer:  fmt.Sprintf("%s://%s", threeportAPIProtocol, threeportAPIEndpoint),
			Kubeconfig: kubeconfigPath,
		}

		// update threeport config to add the new instance and set as current instance
		if threeportInstanceConfigExists {
			for n, instance := range threeportConfig.Instances {
				if instance.Name == createThreeportInstanceName {
					threeportConfig.Instances[n] = *newThreeportInstance
				}
			}
		} else {
			threeportConfig.Instances = append(threeportConfig.Instances, *newThreeportInstance)
		}
		viper.Set("Instances", threeportConfig.Instances)
		viper.Set("CurrentInstance", createThreeportInstanceName)
		viper.WriteConfig()
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
		"provider", "p", "kind", "The infrasture provider to install upon.",
	)
	// this flag will be enabled once production-ready control plane instances
	// are available.
	//CreateControlPlaneCmd.Flags().StringVarP(
	//	&tier,
	//	"tier", "t", threeport.ControlPlaneTierDev, "Determines the level of availability and data retention for the control plane.",
	//)
	CreateControlPlaneCmd.Flags().StringVarP(
		&kubeconfigPath,
		"kubeconfig", "k", "", "Path to kubeconfig needed for kind provider installs (default is ~/.kube/config)",
	)
	CreateControlPlaneCmd.Flags().BoolVar(
		&forceOverwriteConfig,
		"force-overwrite-config", false, "Force the overwrite of an existing Threeport instance config.  Warning: this will erase the connection info for the existing instance.  Only do this if the existing instance has already been deleted and is no longer in use.",
	)
	CreateControlPlaneCmd.Flags().StringVarP(
		&createProviderAccountID,
		"provider-account-id", "a", "", "The provider account ID.  Required if providing a root domain for automated DNS management.",
	)
	CreateControlPlaneCmd.Flags().StringVarP(
		&createRootDomain,
		"root-domain", "d", "", "The root domain name to use for the Threeport API. Requires a public hosted zone in AWS Route53. A subdomain for the Threeport API will be added to the root domain.",
	)
	CreateControlPlaneCmd.Flags().StringVarP(
		&createAdminEmail,
		"admin-email", "e", "", "Email address of control plane admin.  Provided to TLS provider.",
	)
	CreateControlPlaneCmd.Flags().StringVarP(
		&controlPlaneImageRepo,
		"control-plane-image-repo", "i", "", "Alternate image repo to pull threeport control plane images from.",
	)
}

// validateCreateControlPlaneFlags validates flag inputs as needed
func validateCreateControlPlaneFlags(infraProvider, createRootDomain, createProviderAccountID string) error {
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

	if createRootDomain != "" && createProviderAccountID == "" {
		return errors.New(
			"if a root domain is provided for automated DNS management, your cloud provider account ID must also be provided. It is also recommended to provide an admin email, but not required.",
		)
	}

	return nil
}
