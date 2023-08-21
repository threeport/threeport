/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	"github.com/threeport/threeport/internal/cli"
	client "github.com/threeport/threeport/pkg/client/v0"
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
	Run: func(cmd *cobra.Command, args []string) {
		// get threeport config and extract threeport API endpoint
		threeportConfig, err := config.GetThreeportConfig()
		if err != nil {
			cli.Error("failed to get threeport config", err)
			os.Exit(1)
		}
		apiEndpoint, err := threeportConfig.GetThreeportAPIEndpoint()
		if err != nil {
			cli.Error("failed to get threeport API endpoint from config", err)
			os.Exit(1)
		}

		// load gateway definition config
		configContent, err := ioutil.ReadFile(createGatewayDefinitionConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var gatewayDefinitionConfig config.GatewayDefinitionConfig
		if err := yaml.Unmarshal(configContent, &gatewayDefinitionConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}

		// get threeport API client
		cliArgs.AuthEnabled, err = threeportConfig.GetThreeportAuthEnabled()
		if err != nil {
			cli.Error("failed to determine if auth is enabled on threeport API", err)
			os.Exit(1)
		}
		ca, clientCertificate, clientPrivateKey, err := threeportConfig.GetThreeportCertificates()
		if err != nil {
			cli.Error("failed to get threeport certificates from config", err)
			os.Exit(1)
		}
		apiClient, err := client.GetHTTPClient(cliArgs.AuthEnabled, ca, clientCertificate, clientPrivateKey)
		if err != nil {
			cli.Error("failed to create threeport API client", err)
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
	createCmd.AddCommand(CreateGatewayDefinitionCmd)

	CreateGatewayDefinitionCmd.Flags().StringVarP(
		&createGatewayDefinitionConfigPath,
		"config", "c", "", "Path to file with gateway definition config.",
	)
	CreateGatewayDefinitionCmd.MarkFlagRequired("config")
}