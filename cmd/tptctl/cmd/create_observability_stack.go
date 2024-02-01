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

var createObservabilityStackConfigPath string

// CreateObservabilityStackCmd represents the create observability stack command
var CreateObservabilityStackCmd = &cobra.Command{
	Use:     "observability-stack",
	Example: "tptctl create observability-stack --config /path/to/config.yaml",
	Short:   "Create a new observability stack",
	Long: `Create a new observability stack. This command creates a new observability stack definition
and observability stack instance based on the observability stack config.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// load observability stack config
		configContent, err := os.ReadFile(createObservabilityStackConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var observabilityStackConfig config.ObservabilityStack
		if err := yaml.UnmarshalStrict(configContent, &observabilityStackConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}

		// add path to workload config - used to determine relative path from
		// user's working directory to YAML document
		observabilityStackConfig.ObservabilityStack.ObservabilityStackConfigPath = createObservabilityStackConfigPath

		// create observabilityStack
		observabilityStack := observabilityStackConfig.ObservabilityStack
		err = observabilityStack.Create(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to create workload", err)
			os.Exit(1)
		}

		cli.Info(fmt.Sprintf("observability stack definition %s created", observabilityStack.Name))
		cli.Info(fmt.Sprintf("observability stack instance %s created", observabilityStack.Name))
		cli.Complete(fmt.Sprintf("observability stack %s created", observabilityStackConfig.ObservabilityStack.Name))
	},
}

func init() {
	CreateCmd.AddCommand(CreateObservabilityStackCmd)

	CreateObservabilityStackCmd.Flags().StringVarP(
		&createObservabilityStackConfigPath,
		"config", "c", "", "Path to file with workload config.",
	)
	CreateObservabilityStackCmd.MarkFlagRequired("config")
	CreateObservabilityStackCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
