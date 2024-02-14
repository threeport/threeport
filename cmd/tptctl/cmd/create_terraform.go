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

var createTerraformConfigPath string

// CreateTerraformCmd represents the terraform command
var CreateTerraformCmd = &cobra.Command{
	Use:     "terraform",
	Example: "tptctl create terraform --config /path/to/config.yaml",
	Short:   "Create a new terraform",
	Long: `Create a new terraform. This command creates a new terraform definition
and terraform instance based on the terraform config.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// load terraform config
		configContent, err := os.ReadFile(createTerraformConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var terraformConfig config.TerraformConfig
		if err := yaml.UnmarshalStrict(configContent, &terraformConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}

		// add path to terraform config - used to determine relative path from
		// user's working directory to YAML document
		terraformConfig.Terraform.TerraformConfigPath = createTerraformConfigPath

		// create terraform
		terraform := terraformConfig.Terraform
		wd, wi, err := terraform.Create(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to create terraform", err)
			os.Exit(1)
		}

		cli.Info(fmt.Sprintf("terraform definition %s created", *wd.Name))
		cli.Info(fmt.Sprintf("terraform instance %s created", *wi.Name))
		cli.Complete(fmt.Sprintf("terraform %s created", terraformConfig.Terraform.Name))
	},
}

func init() {
	CreateCmd.AddCommand(CreateTerraformCmd)

	CreateTerraformCmd.Flags().StringVarP(
		&createTerraformConfigPath,
		"config", "c", "", "Path to file with terraform config.",
	)
	CreateTerraformCmd.MarkFlagRequired("config")
	CreateTerraformCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
