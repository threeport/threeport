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

var createTerraformInstanceConfigPath string

// CreateTerraformInstanceCmd represents the terraform-instance command
var CreateTerraformInstanceCmd = &cobra.Command{
	Use:          "terraform-instance",
	Example:      "tptctl create terraform-instance --config /path/to/config.yaml",
	Short:        "Create a new terraform instance",
	Long:         `Create a new terraform instance.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// load terraform instance config
		configContent, err := os.ReadFile(createTerraformInstanceConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var terraformInstanceConfig config.TerraformInstanceConfig
		if err := yaml.UnmarshalStrict(configContent, &terraformInstanceConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}

		// create terraform instance
		terraformInstance := terraformInstanceConfig.TerraformInstance
		terraformInstance.TerraformConfigPath = createTerraformInstanceConfigPath
		terraformInst, err := terraformInstance.Create(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to create terraform instance", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("terraform instance %s created\n", *terraformInst.Name))
	},
}

func init() {
	CreateCmd.AddCommand(CreateTerraformInstanceCmd)

	CreateTerraformInstanceCmd.Flags().StringVarP(
		&createTerraformInstanceConfigPath,
		"config", "c", "", "Path to file with terraform instance config.",
	)
	CreateTerraformInstanceCmd.MarkFlagRequired("config")
	CreateTerraformInstanceCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
