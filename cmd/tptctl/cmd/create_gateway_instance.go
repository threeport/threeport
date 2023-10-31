/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
)

var createGatewayInstanceConfigPath string

// CreateGatewayInstanceCmd represents the gateway-instance command
var CreateGatewayInstanceCmd = &cobra.Command{
	Use:          "gateway-instance",
	Example:      "tptctl create gateway-instance --config /path/to/config.yaml",
	Short:        "Create a new gateway instance",
	Long:         `Create a new gateway instance.`,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		// get threeport config and extract threeport API endpoint
		threeportConfig, requestedControlPlane, err := config.GetThreeportConfig(cliArgs.ControlPlaneName)
		if err != nil {
			cli.Error("failed to get threeport config", err)
			os.Exit(1)
		}

		var apiClient *http.Client
		var apiEndpoint string

		apiClient, apiEndpoint = checkContext(cmd)
		if apiClient == nil && apiEndpoint != "" {
			apiEndpoint, err = threeportConfig.GetThreeportAPIEndpoint(requestedControlPlane)
			if err != nil {
				cli.Error("failed to get threeport API endpoint from config", err)
				os.Exit(1)
			}

			apiClient, err = threeportConfig.GetHTTPClient(requestedControlPlane)
			if err != nil {
				cli.Error("failed to create threeport API client", err)
				os.Exit(1)
			}
		}

		// load gateway instance config
		configContent, err := os.ReadFile(createGatewayInstanceConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var gatewayInstanceConfig config.GatewayInstanceConfig
		if err := yaml.Unmarshal(configContent, &gatewayInstanceConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}

		// create gateway instance
		gatewayInstance := gatewayInstanceConfig.GatewayInstance
		wi, err := gatewayInstance.Create(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to create gateway instance", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("gateway instance %s created\n", *wi.Name))
	},
}

func init() {
	CreateCmd.AddCommand(CreateGatewayInstanceCmd)

	CreateGatewayInstanceCmd.Flags().StringVarP(
		&createGatewayInstanceConfigPath,
		"config", "c", "", "Path to file with gateway instance config.",
	)
	CreateGatewayInstanceCmd.MarkFlagRequired("config")
	CreateGatewayInstanceCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
