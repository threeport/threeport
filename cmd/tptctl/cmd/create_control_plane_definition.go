/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
)

var createControlPlaneDefinitionConfigPath string

// CreateControlPlaneDefinitionCmd represents the workload-definition command
var CreateControlPlaneDefinitionCmd = &cobra.Command{
	Use:          "control-plane-definition",
	Example:      "tptctl create control-plane-definition --config /path/to/config.yaml",
	Short:        "Create a new control-plane definition",
	Long:         `Create a new control-plane definition.`,
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

			// get threeport API client
			cliArgs.AuthEnabled, err = threeportConfig.GetThreeportAuthEnabled(requestedControlPlane)
			if err != nil {
				cli.Error("failed to determine if auth is enabled on threeport API", err)
				os.Exit(1)
			}
			ca, clientCertificate, clientPrivateKey, err := threeportConfig.GetThreeportCertificatesForControlPlane(requestedControlPlane)
			if err != nil {
				cli.Error("failed to get threeport certificates from config", err)
				os.Exit(1)
			}
			apiClient, err = client.GetHTTPClient(cliArgs.AuthEnabled, ca, clientCertificate, clientPrivateKey, "")
			if err != nil {
				cli.Error("failed to create threeport API client", err)
				os.Exit(1)
			}
		}

		// load workload definition config
		configContent, err := ioutil.ReadFile(createControlPlaneDefinitionConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var controlPlaneDefinitionConfig config.ControlPlaneDefinitionConfig
		if err := yaml.Unmarshal(configContent, &controlPlaneDefinitionConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}

		// get threeport API client
		apiClient, err := threeportConfig.GetHTTPClient(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get threeport API client", err)
			os.Exit(1)
		}

		// create workload definition
		controlPlaneDefinition := controlPlaneDefinitionConfig.ControlPlaneDefinition
		wd, err := controlPlaneDefinition.Create(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to create control plane definition", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("control plane definition %s created", *wd.Name))
	},
}

func init() {
	CreateCmd.AddCommand(CreateControlPlaneDefinitionCmd)

	CreateControlPlaneDefinitionCmd.Flags().StringVarP(
		&createControlPlaneDefinitionConfigPath,
		"config", "c", "", "Path to file with workload definition config.",
	)
	CreateControlPlaneDefinitionCmd.MarkFlagRequired("config")
}
