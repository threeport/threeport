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

var createHelmWorkloadInstanceConfigPath string

// CreateHelmWorkloadInstanceCmd represents the helm-workload-instance command
var CreateHelmWorkloadInstanceCmd = &cobra.Command{
	Use:          "helm-workload-instance",
	Example:      "tptctl create helm-workload-instance --config /path/to/config.yaml",
	Short:        "Create a new helm workload instance",
	Long:         `Create a new helm workload instance.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// load helm workload instance config
		configContent, err := os.ReadFile(createHelmWorkloadInstanceConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var helmWorkloadInstanceConfig config.HelmWorkloadInstanceConfig
		if err := yaml.UnmarshalStrict(configContent, &helmWorkloadInstanceConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}

		// create helm workload instance
		helmWorkloadInstance := helmWorkloadInstanceConfig.HelmWorkloadInstance
		helmWorkloadInstance.HelmWorkloadConfigPath = createHelmWorkloadInstanceConfigPath
		helmWorkloadInst, err := helmWorkloadInstance.Create(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to create helm workload instance", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("helm workload instance %s created\n", *helmWorkloadInst.Name))
	},
}

func init() {
	CreateCmd.AddCommand(CreateHelmWorkloadInstanceCmd)

	CreateHelmWorkloadInstanceCmd.Flags().StringVarP(
		&createHelmWorkloadInstanceConfigPath,
		"config", "c", "", "Path to file with helm workload instance config.",
	)
	CreateHelmWorkloadInstanceCmd.MarkFlagRequired("config")
	CreateHelmWorkloadInstanceCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
