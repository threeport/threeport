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
	config "github.com/threeport/threeport/pkg/config/v0"
)

var (
	deleteControlPlaneInstanceConfigPath string
	deleteControlPlaneInstanceName       string
)

// DeleteControlPlaneInstanceCmd represents the workload-instance command
var DeleteControlPlaneInstanceCmd = &cobra.Command{
	Use:          "control-plane-instance",
	Example:      "tptctl delete control-plane-instance --config /path/to/config.yaml",
	Short:        "Delete an existing control plane instance",
	Long:         `Delete an existing control plane instance.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// flag validation
		if err := cli.ValidateConfigNameFlags(
			deleteControlPlaneInstanceConfigPath,
			deleteControlPlaneInstanceName,
			"control plane instance",
		); err != nil {
			cli.Error("flag validation failed", err)
			os.Exit(1)
		}

		var controlPlaneInstanceConfig config.ControlPlaneInstanceConfig
		if deleteControlPlaneInstanceConfigPath != "" {
			// load control plane instance config
			configContent, err := ioutil.ReadFile(deleteControlPlaneInstanceConfigPath)
			if err != nil {
				cli.Error("failed to read config file", err)
				os.Exit(1)
			}
			if err := yaml.Unmarshal(configContent, &controlPlaneInstanceConfig); err != nil {
				cli.Error("failed to unmarshal config file yaml content", err)
				os.Exit(1)
			}
		} else {
			controlPlaneInstanceConfig = config.ControlPlaneInstanceConfig{
				ControlPlaneInstance: config.ControlPlaneInstanceValues{
					Name: deleteControlPlaneInstanceName,
				},
			}
		}

		// delete control plane instance
		controlPlaneInstance := controlPlaneInstanceConfig.ControlPlaneInstance
		wi, err := controlPlaneInstance.Delete(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to delete control plane instance", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("control plane instance %s deleted\n", *wi.Name))
	},
}

func init() {
	DeleteCmd.AddCommand(DeleteControlPlaneInstanceCmd)

	DeleteControlPlaneInstanceCmd.Flags().StringVarP(
		&deleteControlPlaneInstanceConfigPath,
		"config", "c", "", "Path to file with control plane instance config.",
	)
	DeleteControlPlaneInstanceCmd.Flags().StringVarP(
		&deleteControlPlaneInstanceName,
		"name", "n", "", "Name of control plane instance.",
	)
}
