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
	cli "github.com/threeport/threeport/pkg/cli/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
	"gopkg.in/yaml.v2"
)

var deleteControlPlaneConfigPath string
var deleteControlPlaneName string

// DeleteControlPlaneCmd represents the delete control-plane command
var DeleteControlPlaneCmd = &cobra.Command{
	Use:          "control-plane",
	Example:      "tptctl delete control-plane --name my-threeport",
	Short:        "Delete an instance of the Threeport control plane",
	Long:         `Delete an instance of the Threeport control plane.`,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {

		// flag validation
		if err := validateDeleteControlPlaneFlags(
			deleteControlPlaneConfigPath,
			deleteControlPlaneName,
		); err != nil {
			cli.Error("flag validation failed", err)
			os.Exit(1)
		}

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

		var controlPlaneConfig config.ControlPlaneConfig
		if deleteControlPlaneConfigPath != "" {
			// load control plane definition config
			configContent, err := ioutil.ReadFile(deleteControlPlaneConfigPath)
			if err != nil {
				cli.Error("failed to read config file", err)
				os.Exit(1)
			}
			if err := yaml.Unmarshal(configContent, &controlPlaneConfig); err != nil {
				cli.Error("failed to unmarshal config file yaml content", err)
				os.Exit(1)
			}
		} else {
			controlPlaneConfig = config.ControlPlaneConfig{
				ControlPlane: config.ControlPlaneValues{
					Name: deleteControlPlaneName,
				},
			}
		}

		// delete control plane
		controlPlane := controlPlaneConfig.ControlPlane
		cd, ci, err := controlPlane.Delete(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to delete control plane", err)
			os.Exit(1)
		}

		cli.Info(fmt.Sprintf("control plane instance %s deleted", *ci.Name))
		cli.Info(fmt.Sprintf("control plane definition %s deleted", *cd.Name))
		cli.Complete(fmt.Sprintf("control plane %s deleted", controlPlaneConfig.ControlPlane.Name))

	},
}

func init() {
	deleteCmd.AddCommand(DeleteControlPlaneCmd)

	DeleteControlPlaneCmd.Flags().StringVarP(
		&deleteControlPlaneConfigPath,
		"config-path", "c", "", "Path to the config used to create the control plane",
	)
	DeleteControlPlaneCmd.Flags().StringVarP(
		&deleteControlPlaneName,
		"name", "n", "", "Name of control plane.",
	)
}

// validateDeleteControlPlaneFlags validates flag inputs as needed.
func validateDeleteControlPlaneFlags(controlPlaneConfigPath, controlPlaneName string) error {
	if controlPlaneConfigPath == "" && controlPlaneName == "" {
		return errors.New("must provide either control plane name or path to config file")
	}

	if controlPlaneConfigPath != "" && controlPlaneName != "" {
		return errors.New("control plane name and path to config file provided - provide only one")
	}

	return nil
}
