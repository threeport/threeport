/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
)

var (
	deleteControlPlaneDefinitionConfigPath string
	deleteControlPlaneDefinitionName       string
)

// DeleteControlPlaneDefinitionCmd represents the control-plane-definition command
var DeleteControlPlaneDefinitionCmd = &cobra.Command{
	Use:          "control-plane-definition",
	Example:      "tptctl delete control-plane-definition --config /path/to/config.yaml",
	Short:        "Delete an existing control plane definition",
	Long:         `Delete an existing control plane definition.`,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {

		// get threeport config and extract threeport API endpoint
		threeportConfig, requestedControlPlane, err := config.GetThreeportConfig(cliArgs.ControlPlaneName)
		if err != nil {
			cli.Error("failed to get threeport config", err)
			os.Exit(1)
		}

		apiEndpoint, err := threeportConfig.GetThreeportAPIEndpoint(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get threeport API endpoint from config", err)
			os.Exit(1)
		}

		// flag validation
		if err := validateDeleteControlPlaneDefinitionFlags(
			deleteControlPlaneDefinitionConfigPath,
			deleteControlPlaneDefinitionName,
		); err != nil {
			cli.Error("flag validation failed", err)
			os.Exit(1)
		}

		var controlPlaneDefinitionConfig config.ControlPlaneDefinitionConfig
		if deleteControlPlaneDefinitionConfigPath != "" {
			// load workload definition config
			configContent, err := ioutil.ReadFile(deleteControlPlaneDefinitionConfigPath)
			if err != nil {
				cli.Error("failed to read config file", err)
				os.Exit(1)
			}
			if err := yaml.Unmarshal(configContent, &controlPlaneDefinitionConfig); err != nil {
				cli.Error("failed to unmarshal config file yaml content", err)
				os.Exit(1)
			}
		} else {
			controlPlaneDefinitionConfig = config.ControlPlaneDefinitionConfig{
				ControlPlaneDefinition: config.ControlPlaneDefinitionValues{
					Name: deleteControlPlaneDefinitionName,
				},
			}
		}

		// get threeport API client
		apiClient, err := threeportConfig.GetHTTPClient(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get threeport API client", err)
			os.Exit(1)
		}

		// delete control plane definition
		controlPlaneDefinition := controlPlaneDefinitionConfig.ControlPlaneDefinition
		wd, err := controlPlaneDefinition.Delete(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to delete control plane definition", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("control plane definition %s deleted", *wd.Name))
	},
}

func init() {
	DeleteCmd.AddCommand(DeleteControlPlaneDefinitionCmd)

	DeleteControlPlaneDefinitionCmd.Flags().StringVarP(
		&deleteControlPlaneDefinitionConfigPath,
		"config", "c", "", "Path to file with control plane definition config.",
	)
	DeleteControlPlaneDefinitionCmd.Flags().StringVarP(
		&deleteControlPlaneDefinitionName,
		"name", "n", "", "Name of control plane definition.",
	)
}

// validateDeleteControlPlaneDefinitionFlags validates flag inputs as needed.
func validateDeleteControlPlaneDefinitionFlags(controlPlaneDefConfigPath, controlPlaneDefName string) error {
	if controlPlaneDefConfigPath == "" && controlPlaneDefName == "" {
		return errors.New("must provide either control plane definition name or path to config file")
	}

	if controlPlaneDefConfigPath != "" && controlPlaneDefName != "" {
		return errors.New("control plane definition name and path to config file provided - provide only one")
	}

	return nil
}
