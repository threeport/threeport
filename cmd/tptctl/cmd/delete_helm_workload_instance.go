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

var (
	deleteHelmWorkloadInstanceConfigPath string
	deleteHelmWorkloadInstanceName       string
)

// DeleteHelmWorkloadInstanceCmd represents the helm-workload-instance command
var DeleteHelmWorkloadInstanceCmd = &cobra.Command{
	Use:          "helm-workload-instance",
	Example:      "tptctl delete helm-workload-instance --config /path/to/config.yaml",
	Short:        "Delete an existing helm workload instance",
	Long:         `Delete an existing helm workload instance.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// flag validation
		if err := cli.ValidateConfigNameFlags(
			deleteHelmWorkloadInstanceConfigPath,
			deleteHelmWorkloadInstanceName,
			"helm workload instance",
		); err != nil {
			cli.Error("flag validation failed", err)
			os.Exit(1)
		}

		var helmWorkloadInstanceConfig config.HelmWorkloadInstanceConfig
		if deleteHelmWorkloadInstanceConfigPath != "" {
			// load helm workload instance config
			configContent, err := os.ReadFile(deleteHelmWorkloadInstanceConfigPath)
			if err != nil {
				cli.Error("failed to read config file", err)
				os.Exit(1)
			}
			if err := yaml.UnmarshalStrict(configContent, &helmWorkloadInstanceConfig); err != nil {
				cli.Error("failed to unmarshal config file yaml content", err)
				os.Exit(1)
			}
		} else {
			helmWorkloadInstanceConfig = config.HelmWorkloadInstanceConfig{
				HelmWorkloadInstance: config.HelmWorkloadInstanceValues{
					Name: deleteHelmWorkloadInstanceName,
				},
			}
		}

		// delete helm workload instance
		helmWorkloadInstance := helmWorkloadInstanceConfig.HelmWorkloadInstance
		_, err := helmWorkloadInstance.Delete(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to delete helm workload instance", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("helm workload instance %s deleted\n", helmWorkloadInstance.Name))
	},
}

func init() {
	DeleteCmd.AddCommand(DeleteHelmWorkloadInstanceCmd)

	DeleteHelmWorkloadInstanceCmd.Flags().StringVarP(
		&deleteHelmWorkloadInstanceConfigPath,
		"config", "c", "", "Path to file with helm workload instance config.",
	)
	DeleteHelmWorkloadInstanceCmd.Flags().StringVarP(
		&deleteHelmWorkloadInstanceName,
		"name", "n", "", "Name of helm workload instance.",
	)
	DeleteHelmWorkloadInstanceCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
