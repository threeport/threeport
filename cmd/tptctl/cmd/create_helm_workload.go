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

var createHelmWorkloadConfigPath string

// CreateHelmWorkloadCmd represents the helm-workload command
var CreateHelmWorkloadCmd = &cobra.Command{
	Use:     "helm-workload",
	Example: "tptctl create helm-workload --config /path/to/config.yaml",
	Short:   "Create a new helm workload",
	Long: `Create a new helm workload. This command creates a new helm workload definition
and helm workload instance based on the helm workload config.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// load helm workload config
		configContent, err := os.ReadFile(createHelmWorkloadConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var helmWorkloadConfig config.HelmWorkloadConfig
		if err := yaml.UnmarshalStrict(configContent, &helmWorkloadConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}

		// add path to helm workload config - used to determine relative path from
		// user's working directory to YAML document
		helmWorkloadConfig.HelmWorkload.HelmWorkloadConfigPath = createHelmWorkloadConfigPath

		// create helm workload
		helmWorkload := helmWorkloadConfig.HelmWorkload
		wd, wi, err := helmWorkload.Create(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to create helm workload", err)
			os.Exit(1)
		}

		cli.Info(fmt.Sprintf("helm workload definition %s created", *wd.Name))
		cli.Info(fmt.Sprintf("helm workload instance %s created", *wi.Name))
		cli.Complete(fmt.Sprintf("helm workload %s created", helmWorkloadConfig.HelmWorkload.Name))
	},
}

func init() {
	CreateCmd.AddCommand(CreateHelmWorkloadCmd)

	CreateHelmWorkloadCmd.Flags().StringVarP(
		&createHelmWorkloadConfigPath,
		"config", "c", "", "Path to file with helm workload config.",
	)
	CreateHelmWorkloadCmd.MarkFlagRequired("config")
	CreateHelmWorkloadCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
