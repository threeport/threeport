/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
)

var configCurrentControlPlaneName string

// ConfigCurrentControlPlaneCmd represents the current-instance command
var ConfigCurrentControlPlaneCmd = &cobra.Command{
	Use:     "current-control-plane",
	Example: "tptctl config current-control-plane --control-plane-name testport",
	Short:   "Set a threeport control plane as the current in-use control plane",
	Long: `Set a threeport control plane as the current in-use control plane.  Once set as
the current control plane all subsequent tptctl commands will apply to that Threeport
control plane.`,
	PreRun:       CommandPreRunFunc,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		// get threeport config
		threeportConfig, _, err := config.GetThreeportConfig(cliArgs.ControlPlaneName)
		if err != nil {
			cli.Error("failed to get threeport config", err)
			os.Exit(1)
		}

		// We try to find the current control plane in the current config
		// If found we set and return
		var genesisControlPlane *config.ControlPlane
		var anyControlPlane *config.ControlPlane
		var currentControlPlane *config.ControlPlane
		for _, controlPlane := range threeportConfig.ControlPlanes {
			if controlPlane.Name == configCurrentControlPlaneName {
				threeportConfig.SetCurrentControlPlane(configCurrentControlPlaneName)
				cli.Info("Control plane found in config")
				cli.Complete(fmt.Sprintf("Threeport control plane %s set as the current control plane", configCurrentControlPlaneName))
				return
			}

			// In case we dont find the control plane we check if we can find the genesis control plane
			// User can traverse from it to find it
			if controlPlane.Genesis {
				genesisControlPlane = &controlPlane
			}

			if controlPlane.Name == threeportConfig.CurrentControlPlane {
				currentControlPlane = &controlPlane
			}

			// In case we dont find the control plane or its genesis instance,
			anyControlPlane = &controlPlane
		}

		cli.Info("Control plane not found in config")

		// get threeport API client
		apiClient, err := threeportConfig.GetHTTPClient(currentControlPlane.Name)
		if err != nil {
			cli.Error("failed to get threeport API client", err)
			os.Exit(1)
		}

		var controlPlaneInstanceToSet *v0.ControlPlaneInstance
		if currentControlPlane != nil {
			cli.Info("Checking if child of current control plane")
			// If the requested control plane is not found already in config,
			// search through the children of the current control plane
			apiEndpoint, err := threeportConfig.GetThreeportAPIEndpoint(currentControlPlane.Name)
			if err != nil {
				cli.Error("failed to get threeport API endpoint from config", err)
				os.Exit(1)
			}

			// get control plane instances
			controlPlaneInstance, err := client.GetControlPlaneInstanceByName(apiClient, apiEndpoint, currentControlPlane.Name)
			if err != nil {
				cli.Error("failed to retrieve current control plane instance", err)
				os.Exit(1)
			}

			for _, controlPlane := range *controlPlaneInstance.Children {
				if *controlPlane.Name == configCurrentControlPlaneName {
					controlPlaneInstanceToSet = &controlPlane
					cli.Info("found requested control plane in children of current control plane")
					updateThreeportConfigWithControlPlaneInstance(apiClient, apiEndpoint, controlPlaneInstanceToSet, threeportConfig)
					// set the current instance
					cli.Complete(fmt.Sprintf("Threeport control plane %s set as the current control plane", configCurrentControlPlaneName))
					return
				}
			}
		} else {
			cli.Warning("Current control plane not found, cannot search children for requested control plane instance.")
		}

		cli.Info("Checking to see genesis control plane info for info exists")
		// If the control plane in the instance is not already present in the config and not a child of the current control plane,
		// we look ensure the genesis control plane information exists and prompt the user to do a manual tree crawl
		if genesisControlPlane != nil {
			cli.Warning(fmt.Sprintf("could not find requested control plane. Try setting current control to the genesis control plane for this instance: %s and traversing the topology of your control planes", genesisControlPlane.Name))
			os.Exit(1)
		}

		if anyControlPlane == nil {
			cli.Warning("could not find any control plane in the requested config. Please add atleast one control plane from the group")
			os.Exit(1)
		}
		apiEndpoint, err := threeportConfig.GetThreeportAPIEndpoint(anyControlPlane.Name)
		if err != nil {
			cli.Error("failed to get threeport API endpoint from config", err)
			os.Exit(1)
		}

		// get control plane instances
		controlPlaneInstanceToSet, err = client.GetGenesisControlPlaneInstance(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to retrieve genesis control plane instance", err)
			os.Exit(1)
		}

		updateThreeportConfigWithControlPlaneInstance(apiClient, apiEndpoint, controlPlaneInstanceToSet, threeportConfig)

		cli.Warning(fmt.Sprintf("could not find requested control plane info. Current control plane set to the genesis control plane for this instance: %s. Try traversing the topology of your control planes", genesisControlPlane.Name))
		os.Exit(1)
	},
}

func updateThreeportConfigWithControlPlaneInstance(apiClient *http.Client, apiEndpoint string, controlPlaneInstanceToSet *v0.ControlPlaneInstance, threeportConfig *config.ThreeportConfig) {
	// Get the corresponding control plane definition
	controlPlaneDefinition, err := client.GetControlPlaneDefinitionByID(apiClient, apiEndpoint, *controlPlaneInstanceToSet.ControlPlaneDefinitionID)
	if err != nil {
		cli.Error("failed to get control plane definition associated with the control plane being set", err)
		os.Exit(1)
	}

	kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByID(apiClient, apiEndpoint, *controlPlaneInstanceToSet.KubernetesRuntimeInstanceID)
	if err != nil {
		cli.Error("failed to get kubernetes runtime instance associated with the control plane being set", err)
		os.Exit(1)
	}

	kubernetesRuntimeDefinition, err := client.GetKubernetesRuntimeDefinitionByID(apiClient, apiEndpoint, *kubernetesRuntimeInstance.KubernetesRuntimeDefinitionID)
	if err != nil {
		cli.Error("failed to get kubernetes runtime definition associated with the control plane being set", err)
		os.Exit(1)
	}

	var threeportControlPlaneConfig *config.ControlPlane

	if !*controlPlaneDefinition.AuthEnabled {
		threeportControlPlaneConfig = &config.ControlPlane{
			Name:        *controlPlaneInstanceToSet.Name,
			APIServer:   *controlPlaneInstanceToSet.ApiServerEndpoint,
			AuthEnabled: *controlPlaneDefinition.AuthEnabled,
			Provider:    *kubernetesRuntimeDefinition.InfraProvider,
		}
	} else {

		// we construct the instance info for the threeport config and add it
		threeportControlPlaneConfig = &config.ControlPlane{
			Name:        *controlPlaneInstanceToSet.Name,
			APIServer:   *controlPlaneInstanceToSet.ApiServerEndpoint,
			AuthEnabled: *controlPlaneDefinition.AuthEnabled,
			CACert:      *controlPlaneInstanceToSet.CACert,
			Provider:    *kubernetesRuntimeDefinition.InfraProvider,
			Credentials: []config.Credential{
				{
					Name:       *controlPlaneInstanceToSet.Name,
					ClientCert: *controlPlaneInstanceToSet.ClientCert,
					ClientKey:  *controlPlaneInstanceToSet.ClientKey,
				},
			},
		}
	}

	if err := config.UpdateThreeportConfig(threeportConfig, threeportControlPlaneConfig); err != nil {
		cli.Error("failed to update threeport config", err)
		os.Exit(1)
	}
}

func init() {
	ConfigCmd.AddCommand(ConfigCurrentControlPlaneCmd)

	ConfigCurrentControlPlaneCmd.Flags().StringVarP(
		&configCurrentControlPlaneName,
		"control-plane-name", "n", "", "The name of the Control plane to set as current.",
	)
	ConfigCurrentControlPlaneCmd.MarkFlagRequired("control-plane-name")
}
