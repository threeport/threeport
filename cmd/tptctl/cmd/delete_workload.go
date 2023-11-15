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
	deleteWorkloadConfigPath string
	deleteWorkloadName       string
)

// DeleteWorkloadCmd represents the workload command
var DeleteWorkloadCmd = &cobra.Command{
	Use:     "workload",
	Example: "tptctl delete workload --config /path/to/config.yaml",
	Short:   "Delete an existing workload",
	Long: `Delete an existing workload. This command deletes an existing workload definition
and workload instance based on the workload config or name.`,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {

		// get threeport config and extract threeport API endpoint
		threeportConfig, requestedControlPlane, err := config.GetThreeportConfig(cliArgs.ControlPlaneName)
		if err != nil {
			cli.Error("failed to get threeport config", err)
			os.Exit(1)
		}

		apiEndpoint, err := threeportConfig.GetThreeportAPIEndpoint(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get threeport API endpoint from config", err)
			os.Exit(1)
		}

		// flag validation
		if err := validateDeleteWorkloadFlags(
			deleteWorkloadConfigPath,
			deleteWorkloadName,
		); err != nil {
			cli.Error("flag validation failed", err)
			os.Exit(1)
		}

		var workloadConfig config.WorkloadConfig
		if deleteWorkloadConfigPath != "" {
			// load workload definition config
			configContent, err := os.ReadFile(deleteWorkloadConfigPath)
			if err != nil {
				cli.Error("failed to read config file", err)
				os.Exit(1)
			}
			if err := yaml.Unmarshal(configContent, &workloadConfig); err != nil {
				cli.Error("failed to unmarshal config file yaml content", err)
				os.Exit(1)
			}
		} else {
			workloadConfig = config.WorkloadConfig{
				Workload: config.WorkloadValues{
					Name: deleteWorkloadName,
				},
			}
		}

		// get threeport API client
		apiClient, err := threeportConfig.GetHTTPClient(requestedControlPlane)
		if err != nil {
			cli.Error("failed to get threeport API client", err)
			os.Exit(1)
		}

		// delete workload
		workload := workloadConfig.Workload
		wd, wi, err := workload.Delete(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to delete workload", err)
			os.Exit(1)
		}

		cli.Info(fmt.Sprintf("workload instance %s deleted", *wi.Name))
		cli.Info(fmt.Sprintf("workload definition %s deleted", *wd.Name))
		cli.Complete(fmt.Sprintf("workload %s deleted", workloadConfig.Workload.Name))
	},
}

func init() {
	deleteCmd.AddCommand(DeleteWorkloadCmd)

	DeleteWorkloadCmd.Flags().StringVarP(
		&deleteWorkloadConfigPath,
		"config", "c", "", "Path to file with workload config.",
	)
	DeleteWorkloadCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}

// validateDeleteWorkloadFlags validates flag inputs as needed.
func validateDeleteWorkloadFlags(workloadConfigPath, workloadName string) error {
	if workloadConfigPath == "" && workloadName == "" {
		return errors.New("must provide either workload name or path to config file")
	}

	if workloadConfigPath != "" && workloadName != "" {
		return errors.New("workload name and path to config file provided - provide only one")
	}

	return nil
}
