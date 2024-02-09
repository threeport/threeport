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
	describeWorkloadDefinitionConfigPath string
	describeWorkloadDefinitionName       string
	describeWorkloadDefinitionYamlFile   string
)

// DescribeWorkloadDefinitionCmd represents the workload-definition command
var DescribeWorkloadDefinitionCmd = &cobra.Command{
	Use:          "workload-definition",
	Example:      "tptctl describe workload-definition",
	Short:        "Describe a workload definition",
	Long:         `Describe a workload definition.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// flag validation
		if err := cli.ValidateConfigNameFlags(
			describeWorkloadDefinitionConfigPath,
			describeWorkloadDefinitionName,
			"workload definition",
		); err != nil {
			cli.Error("flag validation failed", err)
			os.Exit(1)
		}

		var workloadDefinitionConfig config.WorkloadDefinitionConfig
		if describeWorkloadDefinitionConfigPath != "" {
			// load workload definition config
			configContent, err := os.ReadFile(describeWorkloadDefinitionConfigPath)
			if err != nil {
				cli.Error("failed to read config file", err)
				os.Exit(1)
			}
			if err := yaml.Unmarshal(configContent, &workloadDefinitionConfig); err != nil {
				cli.Error("failed to unmarshal config file yaml content", err)
				os.Exit(1)
			}
		} else {
			workloadDefinitionConfig = config.WorkloadDefinitionConfig{
				WorkloadDefinition: config.WorkloadDefinitionValues{
					Name: describeWorkloadDefinitionName,
				},
			}
		}

		// describe workload definition
		workloadDefinition := workloadDefinitionConfig.WorkloadDefinition
		workloadStatus, err := workloadDefinition.Describe(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to describe workload definition", err)
			os.Exit(1)
		}

		// output describe details
		cli.Info(fmt.Sprintf(
			"Workload Definition Name: %s",
			workloadDefinitionConfig.WorkloadDefinition.Name,
		))
		cli.Info("Derived Workload Instances:")
		for _, workloadInst := range *workloadStatus.WorkloadInstances {
			fmt.Printf("      * %s\n", *workloadInst.Name)
		}
		if describeWorkloadDefinitionYamlFile == "" {
			cli.Info("Workload Definition YAML Document:")
			fmt.Println(workloadStatus.YamlDocument)
		} else {
			if err := os.WriteFile(
				describeWorkloadDefinitionYamlFile,
				[]byte(workloadStatus.YamlDocument),
				0644,
			); err != nil {
				cli.Error("failed to write YAML document file: %w", err)
				os.Exit(1)
			}
			cli.Info(fmt.Sprintf(
				"Workload Definition YAML Document written to file %s", describeWorkloadDefinitionYamlFile,
			))
		}
	},
}

func init() {
	DescribeCmd.AddCommand(DescribeWorkloadDefinitionCmd)

	DescribeWorkloadDefinitionCmd.Flags().StringVarP(
		&describeWorkloadDefinitionConfigPath,
		"config", "c", "", "Path to file with workload definition config.",
	)
	DescribeWorkloadDefinitionCmd.Flags().StringVarP(
		&describeWorkloadDefinitionName,
		"name", "n", "", "Name of workload definition.",
	)
	DescribeWorkloadDefinitionCmd.Flags().StringVarP(
		&describeWorkloadDefinitionYamlFile,
		"yaml-output-file", "y", "", "Path to file to write YAML document to.",
	)
	DescribeWorkloadDefinitionCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
