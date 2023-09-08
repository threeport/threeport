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

	cli "github.com/threeport/threeport/pkg/cli/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
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

		// load gateway instance config
		configContent, err := ioutil.ReadFile(createGatewayInstanceConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var gatewayInstanceConfig config.GatewayInstanceConfig
		if err := yaml.Unmarshal(configContent, &gatewayInstanceConfig); err != nil {
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
	createCmd.AddCommand(CreateGatewayInstanceCmd)

	CreateGatewayInstanceCmd.Flags().StringVarP(
		&createGatewayInstanceConfigPath,
		"config", "c", "", "Path to file with gateway instance config.",
	)
	CreateGatewayInstanceCmd.MarkFlagRequired("config")
}
