/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
)

var deleteHelmWorkloadConfigPath string

// DeleteHelmWorkloadCmd represents the helm-workload command
var DeleteHelmWorkloadCmd = &cobra.Command{
	Use:     "helm-workload",
	Example: "tptctl delete helm-workload --config /path/to/config.yaml",
	Short:   "Delete an existing helm workload",
	Long: `Delete an existing helm workload. This command deletes an existing helm workload definition
and helm workload instance based on the helm workload config or name.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// flag validation
		if deleteHelmWorkloadConfigPath == "" {
			cli.Error("flag validation failed", errors.New("config file path is required"))
		}

		var helmWorkloadConfig config.HelmWorkloadConfig
		// load helm workload definition config
		configContent, err := os.ReadFile(deleteHelmWorkloadConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		if err := yaml.Unmarshal(configContent, &helmWorkloadConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}
		// add path to helm workload config - used to determine relative path from
		// user's working directory to YAML document
		helmWorkloadConfig.HelmWorkload.HelmWorkloadConfigPath = deleteHelmWorkloadConfigPath

		// delete helm workload
		helmWorkload := helmWorkloadConfig.HelmWorkload
		_, _, err = helmWorkload.Delete(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to delete helm workload", err)
			os.Exit(1)
		}

		cli.Info(fmt.Sprintf("helm workload instance %s deleted", helmWorkload.Name))
		cli.Info(fmt.Sprintf("helm workload definition %s deleted", helmWorkload.Name))
		cli.Complete(fmt.Sprintf("helm workload %s deleted", helmWorkloadConfig.HelmWorkload.Name))
	},
}

func init() {
	DeleteCmd.AddCommand(DeleteHelmWorkloadCmd)

	DeleteHelmWorkloadCmd.Flags().StringVarP(
		&deleteHelmWorkloadConfigPath,
		"config", "c", "", "Path to file with helm workload config.",
	)
	DeleteHelmWorkloadCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
