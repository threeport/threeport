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

	"github.com/threeport/threeport/internal/tptctl/config"
	"github.com/threeport/threeport/internal/tptctl/output"
	"github.com/threeport/threeport/internal/tptctl/provider"
)

var (
	createThreeportInstanceName string
	createRootDomain            string
	createProviderAccountID     string
	createAdminEmail            string
	forceOverwriteConfig        bool
	infraProvider               string
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
		threeportConfig := &config.ThreeportConfig{}
		if err := viper.Unmarshal(threeportConfig); err != nil {
			output.Error("Failed to get Threeport config", err)
			os.Exit(1)
		}

		// check threeport config for exisiting instance
		threeportInstanceConfigExists := false
		for _, instance := range threeportConfig.Instances {
			if instance.Name == createThreeportInstanceName {
				threeportInstanceConfigExists = true
				if !forceOverwriteConfig {
					output.Error(
						"Interupted creation of Threeport instance",
						errors.New(fmt.Sprintf("instance of Threeport with name %s already exists", instance.Name)),
					)
					output.Info("If you wish to overwrite the existing config use --force-overwrite-config flag")
					output.Warning("You will lose the ability to connect to the existing Threeport instance if it still exists")
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
			output.Error("Flag validation failed", err)
			os.Exit(1)
		}

		// the control plane object provides the config for installing on the
		// provider
		controlPlane := provider.NewControlPlane()
		controlPlane.InstanceName = createThreeportInstanceName
		if createRootDomain != "" {
			controlPlane.RootDomainName = createRootDomain
			controlPlane.ProviderAccountID = createProviderAccountID
			controlPlane.AdminEmail = createAdminEmail
		}

		// determine infra provider and create control plane
		var controlPlaneErr error
		var threeportAPIEndpoint string
		switch infraProvider {
		case "kind":
			if err := controlPlane.CreateControlPlaneOnKind(providerConfigDir); err != nil {
				controlPlaneErr = fmt.Errorf("failed to install control plane on kind: %w", err)
				threeportAPIEndpoint = fmt.Sprintf("%s://%s:%s",
					provider.KindThreeportAPIProtocol, provider.KindThreeportAPIHostname,
					provider.KindThreeportAPIPort)
			}
		case "eks":
			tpapiEndpoint, err := controlPlane.CreateControlPlaneOnEKS(providerConfigDir)
			if err != nil {
				controlPlaneErr = fmt.Errorf("failed to install control plane on EKS: %w", err)
			}
			threeportAPIEndpoint = tpapiEndpoint
		default:
			output.Error("Unrecognized infra provider",
				errors.New(fmt.Sprintf("infra provider %s not supported", infraProvider)))
			os.Exit(1)
		}

		// create threeport config for new instance
		newThreeportInstance := &config.Instance{
			Name:      createThreeportInstanceName,
			Provider:  infraProvider,
			APIServer: threeportAPIEndpoint,
			//APIServer: install.GetThreeportAPIEndpoint(),
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
		output.Info("Threeport config updated")

		if controlPlaneErr != nil {
			output.Error("Problem encountered installing control plane", controlPlaneErr)
		} else {
			output.Complete(fmt.Sprintf("Threeport instance %s created", createThreeportInstanceName))
		}
	},
}

func init() {
	createCmd.AddCommand(CreateControlPlaneCmd)
	CreateControlPlaneCmd.Flags().StringVarP(&infraProvider,
		"provider", "p", "kind", "the infrasture provider to install upon")
	CreateControlPlaneCmd.Flags().StringVarP(&createThreeportInstanceName,
		"name", "n", "", "name of control plane instance")
	CreateControlPlaneCmd.MarkFlagRequired("name")
	CreateControlPlaneCmd.Flags().BoolVar(
		&forceOverwriteConfig, "force-overwrite-config", false,
		"force the overwrite of an existing Threeport instance config.  Warning: this will erase the connection info for the existing instance.  Only do this if the existing instance has already been deleted and is no longer in use.")
	CreateControlPlaneCmd.Flags().StringVarP(&createRootDomain,
		"root-domain", "d", "",
		"the root domain name to use for the Threeport API. Requires a public hosted zone in AWS Route53. A subdomain for the Threeport API will be added to the root domain.")
	CreateControlPlaneCmd.Flags().StringVarP(&createProviderAccountID,
		"provider-account-id", "a", "",
		"the provider account ID.  Required if providing a root domain for automated DNS management.")
	CreateControlPlaneCmd.Flags().StringVarP(&createAdminEmail,
		"admin-email", "e", "",
		"email address of control plane admin.  Provided to TLS provider.")
}

// validateCreateControlPlaneFlags validates flag inputs as needed
func validateCreateControlPlaneFlags(infraProvider, createRootDomain, createProviderAccountID string) error {
	allowedInfraProviders := []string{"kind", "eks"}
	matched := false
	for _, prov := range allowedInfraProviders {
		if infraProvider == prov {
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
