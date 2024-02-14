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

var (
	deleteTerraformConfigPath string
)

// DeleteTerraformCmd represents the terraform command
var DeleteTerraformCmd = &cobra.Command{
	Use:     "terraform",
	Example: "tptctl delete terraform --config /path/to/config.yaml",
	Short:   "Delete an existing terraform resource deployment",
	Long: `Delete an existing terraform resource deployment. This command deletes an existing
terraform definition and terraform instance based on the terraform config or name.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		// flag validation
		if deleteTerraformConfigPath == "" {
			cli.Error("flag validation failed", errors.New("config file path is required"))
		}

		var terraformConfig config.TerraformConfig
		// load terraform definition config
		configContent, err := os.ReadFile(deleteTerraformConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		if err := yaml.UnmarshalStrict(configContent, &terraformConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}

		// delete terraform
		terraform := terraformConfig.Terraform
		_, _, err = terraform.Delete(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to delete terraform", err)
			os.Exit(1)
		}

		cli.Info(fmt.Sprintf("terraform instance %s deleted", terraform.Name))
		cli.Info(fmt.Sprintf("terraform definition %s deleted", terraform.Name))
		cli.Complete(fmt.Sprintf("terraform %s deleted", terraformConfig.Terraform.Name))
	},
}

func init() {
	DeleteCmd.AddCommand(DeleteTerraformCmd)

	DeleteTerraformCmd.Flags().StringVarP(
		&deleteTerraformConfigPath,
		"config", "c", "", "Path to file with terraform config.",
	)
	DeleteTerraformCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
