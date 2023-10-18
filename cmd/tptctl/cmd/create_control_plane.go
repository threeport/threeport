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

var createControlPlaneConfigPath string

// CreateControlPlaneCmd represents the create threeport command
var CreateControlPlaneCmd = &cobra.Command{
	Use:          "control-plane",
	Example:      "tptctl create control-plane --c my-threeport.yaml",
	Short:        "Create a new Threeport control plane",
	Long:         `Create a new control plane.`,
	SilenceUsage: true,
	PreRun: func(cmd *cobra.Command, args []string) {
		switch cliArgs.InfraProvider {
		case v0.KubernetesRuntimeInfraProviderEKS:
			cmd.MarkFlagRequired("aws-region")
		}
	},
	Run: func(cmd *cobra.Command, args []string) {
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
		apiClient, err := client.GetHTTPClient(cliArgs.AuthEnabled, ca, clientCertificate, clientPrivateKey, "")
		if err != nil {
			cli.Error("failed to create https client", err)
			os.Exit(1)
		}

		// load control plane config
		configContent, err := ioutil.ReadFile(createControlPlaneConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var controlPlaneConfig config.ControlPlaneConfig
		if err := yaml.Unmarshal(configContent, &controlPlaneConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}
		// create control plane
		controlPlane := controlPlaneConfig.ControlPlane
		cd, ci, err := controlPlane.Create(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to create control plane", err)
			os.Exit(1)
		}

		cli.Info(fmt.Sprintf("control plane definition %s created", *cd.Name))
		cli.Info(fmt.Sprintf("control plane instance %s created", *ci.Name))
		cli.Complete(fmt.Sprintf("control plane %s created", controlPlane.Name))

	},
}

func init() {
	createCmd.AddCommand(CreateControlPlaneCmd)
	CreateControlPlaneCmd.Flags().StringVarP(
		&createControlPlaneConfigPath,
		"config", "c", "", "Path to file with control plane config.",
	)
	CreateControlPlaneCmd.MarkFlagRequired("config")
}
