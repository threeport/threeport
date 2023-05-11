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
	client "github.com/threeport/threeport/pkg/client/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
)

var (
	deleteWorkloadDefinitionConfigPath string
	deleteWorkloadDefinitionName       string
)

// DeleteWorkloadDefinitionCmd represents the workload-definition command
var DeleteWorkloadDefinitionCmd = &cobra.Command{
	Use:          "workload-definition",
	Example:      "tptctl delete workload-definition --config /path/to/config.yaml",
	Short:        "Delete an existing workload definition",
	Long:         `Delete as existing workload definition.`,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {

		// get threeport config and extract threeport API endpoint
		threeportConfig, err := config.GetThreeportConfig()
		if err != nil {
			cli.Error("failed to get threeport config", err)
		}

		apiEndpoint, err := threeportConfig.GetThreeportAPIEndpoint()
		if err != nil {
			cli.Error("failed to get threeport API endpoint from config", err)
			os.Exit(1)
		}

		// flag validation
		if err := validateDeleteWorkloadDefinitionFlags(
			deleteWorkloadDefinitionConfigPath,
			deleteWorkloadDefinitionName,
		); err != nil {
			cli.Error("flag validation failed", err)
			os.Exit(1)
		}

		var workloadDefinitionConfig config.WorkloadDefinitionConfig
		if deleteWorkloadDefinitionConfigPath != "" {
			// load workload definition config
			configContent, err := ioutil.ReadFile(deleteWorkloadDefinitionConfigPath)
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
					Name: deleteWorkloadDefinitionName,
				},
			}
		}

		// get threeport API client
		ca, clientCertificate, clientPrivateKey, err := threeportConfig.GetThreeportCertificates()
		if err != nil {
			cli.Error("failed to get threeport certificates from config", err)
			os.Exit(1)
		}
		apiClient, err := client.GetHTTPClient(authEnabled, ca, clientCertificate, clientPrivateKey)
		if err != nil {
			cli.Error("failed to create https client", err)
			os.Exit(1)
		}

		// delete workload definition
		workloadDefinition := workloadDefinitionConfig.WorkloadDefinition
		wd, err := workloadDefinition.Delete(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to delete workload definition", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("workload definition %s deleted", *wd.Name))
	},
}

func init() {
	deleteCmd.AddCommand(DeleteWorkloadDefinitionCmd)

	DeleteWorkloadDefinitionCmd.Flags().StringVarP(
		&deleteWorkloadDefinitionConfigPath,
		"config", "c", "", "Path to file with workload definition config.",
	)
	DeleteWorkloadDefinitionCmd.Flags().StringVarP(
		&deleteWorkloadDefinitionName,
		"name", "n", "", "Name of workload definition.",
	)
}

// validateDeleteWorkloadDefinitionFlags validates flag inputs as needed.
func validateDeleteWorkloadDefinitionFlags(workloadDefConfigPath, workloadDefName string) error {
	if workloadDefConfigPath == "" && workloadDefName == "" {
		return errors.New("must provide either workload definition name or path to config file")
	}

	if workloadDefConfigPath != "" && workloadDefName != "" {
		return errors.New("workload definition name and path to config file provided - provide only one")
	}

	return nil
}
