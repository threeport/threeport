/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	"github.com/threeport/threeport/internal/cli"
	config "github.com/threeport/threeport/pkg/config/v0"
)

var (
	deleteWorkloadInstanceConfigPath string
	deleteWorkloadInstanceName       string
)

// DeleteWorkloadInstanceCmd represents the workload-instance command
var DeleteWorkloadInstanceCmd = &cobra.Command{
	Use:          "workload-instance",
	Example:      "tptctl delete workload-instance --config /path/to/config.yaml",
	Short:        "Delete an existing workload instance",
	Long:         `Delete an existing workload instance.`,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		// get threeport config and extract threeport API endpoint
		threeportConfig := config.GetThreeportConfig()

		apiEndpoint, err := threeportConfig.GetThreeportAPIEndpoint()
		if err != nil {
			cli.Error("failed to get threeport API endpoint from config", err)
			os.Exit(1)
		}

		// flag validation
		if err := validateDeleteWorkloadInstanceFlags(
			deleteWorkloadInstanceConfigPath,
			deleteWorkloadInstanceName,
		); err != nil {
			cli.Error("flag validation failed", err)
			os.Exit(1)
		}

		var workloadInstanceConfig config.WorkloadInstanceConfig
		if deleteWorkloadInstanceConfigPath != "" {
			// load workload instance config
			configContent, err := ioutil.ReadFile(deleteWorkloadInstanceConfigPath)
			if err != nil {
				cli.Error("failed to read config file", err)
				os.Exit(1)
			}
			if err := yaml.Unmarshal(configContent, &workloadInstanceConfig); err != nil {
				cli.Error("failed to unmarshal config file yaml content", err)
				os.Exit(1)
			}
		} else {
			workloadInstanceConfig = config.WorkloadInstanceConfig{
				WorkloadInstance: config.WorkloadInstanceValues{
					Name: deleteWorkloadInstanceName,
				},
			}
		}

		apiClient, err := config.GetHTTPClient(authEnabled)
		if err != nil {
			fmt.Errorf("failed to create https client: %w", err)
			os.Exit(1)
		}

		// delete workload instance
		workloadInstance := workloadInstanceConfig.WorkloadInstance
		wi, err := workloadInstance.Delete(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to delete workload", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("workload instance %s deleted\n", *wi.Name))
	},
}

func init() {
	deleteCmd.AddCommand(DeleteWorkloadInstanceCmd)

	DeleteWorkloadInstanceCmd.Flags().StringVarP(
		&deleteWorkloadInstanceConfigPath,
		"config", "c", "", "Path to file with workload instance config.",
	)
	DeleteWorkloadInstanceCmd.Flags().StringVarP(
		&deleteWorkloadInstanceName,
		"name", "n", "", "Name of workload instance.",
	)
}

// validateCreateControlPlaneFlags validates flag inputs as needed.
func validateDeleteWorkloadInstanceFlags(workloadInstConfigPath, workloadInstName string) error {
	if workloadInstConfigPath == "" && workloadInstName == "" {
		return errors.New("must provide either workload instance name or path to config file")
	}

	if workloadInstConfigPath != "" && workloadInstName != "" {
		return errors.New("workload instance name and path to config file provided - provide only one")
	}

	return nil
}
