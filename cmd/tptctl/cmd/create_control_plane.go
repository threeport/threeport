/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/threeport/threeport/internal/cli"
	"github.com/threeport/threeport/internal/kube"
	"github.com/threeport/threeport/internal/provider"
	"github.com/threeport/threeport/internal/threeport"
	"github.com/threeport/threeport/internal/tptctl"
	v0 "github.com/threeport/threeport/pkg/api/v0"
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
)

// CreateControlPlaneCmd represents the create threeport command
var CreateControlPlaneCmd = &cobra.Command{
	Use:          "control-plane",
	Example:      "tptctl create control-plane",
	Short:        "Create a new instance of the Threeport control plane",
	Long:         `Create a new instance of the Threeport control plane.`,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		// get threeport config
		threeportConfig := &tptctl.ThreeportConfig{}
		if err := viper.Unmarshal(threeportConfig); err != nil {
			cli.Error("Failed to get Threeport config", err)
			os.Exit(1)
		}

		// check threeport config for exisiting instance
		threeportInstanceConfigExists := false
		for _, instance := range threeportConfig.Instances {
			if instance.Name == createThreeportInstanceName {
				threeportInstanceConfigExists = true
				if !forceOverwriteConfig {
					cli.Error(
						"Interupted creation of Threeport instance",
						errors.New(fmt.Sprintf("instance of Threeport with name %s already exists", instance.Name)),
					)
					cli.Info("If you wish to overwrite the existing config use --force-overwrite-config flag")
					cli.Warning("You will lose the ability to connect to the existing Threeport instance if it still exists")
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
			cli.Error("Flag validation failed", err)
			os.Exit(1)
		}

		// configure the control plane
		controlPlane := threeport.ControlPlane{
			InfraProvider: threeport.ControlPlaneInfraProvider(infraProvider),
			Tier:          tier,
		}

		// configure the infra provider
		var controlPlaneInfra provider.ControlPlaneInfra
		switch controlPlane.InfraProvider {
		case threeport.ControlPlaneInfraProviderKind:
			// get kubeconfig to use for kind cluster
			if kubeconfigPath == "" {
				k, err := kube.DefaultKubeconfig()
				if err != nil {
					cli.Error("Failed to get default kubeconfig path", err)
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
		var controlPlaneErr error
		kubeConnectionInfo, err := controlPlaneInfra.Create()
		if err != nil {
			controlPlaneErr = fmt.Errorf("failed to get create control plane infra for threeport: %w", err)
		}

		// the cluster instance is the default compute space cluster to be added
		// to the API
		clusterInstName := fmt.Sprintf("compute-space-%s-0", createThreeportInstanceName)
		clusterInstance := v0.ClusterInstance{
			Instance: v0.Instance{
				Name: &clusterInstName,
			},
			APIEndpoint:   &kubeConnectionInfo.APIEndpoint,
			CACertificate: &kubeConnectionInfo.CACertificate,
			Certificate:   &kubeConnectionInfo.Certificate,
			Key:           &kubeConnectionInfo.Key,
		}

		// create a client to connect to kind cluster kube API
		dynamicKubeClient, mapper, err := kube.GetClient(&clusterInstance, false)
		if err != nil {
			cli.Error("failed to get a Kubernetes client and mapper", err)
			os.Exit(1)
		}

		// install the threeport control plane dependencies
		if err := threeport.InstallThreeportControlPlaneDependencies(dynamicKubeClient, mapper); err != nil {
			cli.Error("failed to install threeport control plane dependencies", err)
			os.Exit(1)
		}

		// install the threeport control plane API and controllers
		if err := threeport.InstallThreeportControlPlaneComponents(
			dynamicKubeClient,
			mapper,
			false,
			"localhost",
		); err != nil {
			cli.Error("failed to install threeport control plane components", err)
			os.Exit(1)
		}

		// create threeport config for new instance
		newThreeportInstance := &tptctl.Instance{
			Name:       createThreeportInstanceName,
			Provider:   infraProvider,
			APIServer:  kubeConnectionInfo.APIEndpoint,
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
		cli.Info("Threeport config updated")

		if controlPlaneErr != nil {
			cli.Error("Problem encountered installing control plane", controlPlaneErr)
			os.Exit(1)
		} else {
			cli.Complete(fmt.Sprintf("Threeport instance %s created", createThreeportInstanceName))
		}
	},
}

func init() {
	createCmd.AddCommand(CreateControlPlaneCmd)
	CreateControlPlaneCmd.Flags().StringVarP(&createThreeportInstanceName,
		"name", "n", "", "name of control plane instance")
	CreateControlPlaneCmd.Flags().StringVarP(&infraProvider,
		"provider", "p", "kind", "the infrasture provider to install upon")
	// this flag will be enabled once production-ready control plane instances
	// are available.
	//CreateControlPlaneCmd.Flags().StringVarP(&tier,
	//	"tier", "t", threeport.ControlPlaneTierDev, "determines the level of availability and data retention for the control plane")
	CreateControlPlaneCmd.MarkFlagRequired("name")
	CreateControlPlaneCmd.Flags().StringVarP(&kubeconfigPath,
		"kubeconfig", "k", "", "path to kubeconfig needed for kind provider installs - default is ~/.kube/config")
	CreateControlPlaneCmd.Flags().BoolVar(
		&forceOverwriteConfig, "force-overwrite-config", false,
		"force the overwrite of an existing Threeport instance config.  Warning: this will erase the connection info for the existing instance.  Only do this if the existing instance has already been deleted and is no longer in use.")
	CreateControlPlaneCmd.Flags().StringVarP(&createProviderAccountID,
		"provider-account-id", "a", "",
		"the provider account ID.  Required if providing a root domain for automated DNS management.")
	CreateControlPlaneCmd.Flags().StringVarP(&createRootDomain,
		"root-domain", "d", "",
		"the root domain name to use for the Threeport API. Requires a public hosted zone in AWS Route53. A subdomain for the Threeport API will be added to the root domain.")
	CreateControlPlaneCmd.Flags().StringVarP(&createAdminEmail,
		"admin-email", "e", "",
		"email address of control plane admin.  Provided to TLS provider.")
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
		return errors.New(fmt.Sprintf("invalid provider value '%s' - must be one of %s",
			infraProvider, allowedInfraProviders))
	}

	if createRootDomain != "" && createProviderAccountID == "" {
		return errors.New(
			"if a root domain is provided for automated DNS management, your cloud provider account ID must also be provided. It is also recommended to provide an admin email, but not required.")
	}

	return nil
}
