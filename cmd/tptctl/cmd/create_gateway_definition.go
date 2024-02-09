/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
)

var createGatewayDefinitionConfigPath string

// CreateGatewayDefinitionCmd represents the gateway-definition command
var CreateGatewayDefinitionCmd = &cobra.Command{
	Use:          "gateway-definition",
	Example:      "tptctl create gateway-definition --config /path/to/config.yaml",
	Short:        "Create a new gateway definition",
	Long:         `Create a new gateway definition.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// load gateway definition config
		configContent, err := os.ReadFile(createGatewayDefinitionConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var gatewayDefinitionConfig config.GatewayDefinitionConfig
		if err := yaml.UnmarshalStrict(configContent, &gatewayDefinitionConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}

		// create gateway definition
		gatewayDefinition := gatewayDefinitionConfig.GatewayDefinition
		wd, err := gatewayDefinition.Create(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to create gateway definition", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("gateway definition %s created", *wd.Name))
	},
}

func init() {
	CreateCmd.AddCommand(CreateGatewayDefinitionCmd)

	CreateGatewayDefinitionCmd.Flags().StringVarP(
		&createGatewayDefinitionConfigPath,
		"config", "c", "", "Path to file with gateway definition config.",
	)
	CreateGatewayDefinitionCmd.MarkFlagRequired("config")
	CreateGatewayDefinitionCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
