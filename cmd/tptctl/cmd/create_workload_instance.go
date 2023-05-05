/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"

	"github.com/threeport/threeport/internal/cli"
	clientInternal "github.com/threeport/threeport/internal/client"
	configInternal "github.com/threeport/threeport/internal/config"
	config "github.com/threeport/threeport/pkg/config/v0"
)

var createWorkloadInstanceConfigPath string

// CreateWorkloadInstanceCmd represents the workload-instance command
var CreateWorkloadInstanceCmd = &cobra.Command{
	Use:          "workload-instance",
	Example:      "tptctl create workload-instance --config /path/to/config.yaml",
	Short:        "Create a new workload instance",
	Long:         `Create a new workload instance.`,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {

		// get threeport config and extract threeport API endpoint
		threeportConfig, err := configInternal.GetThreeportConfig()
		if err != nil {
			cli.Error("failed to get threeport config", err)
		}

		apiEndpoint, err := threeportConfig.GetThreeportAPIEndpoint()
		if err != nil {
			cli.Error("failed to get threeport API endpoint from config", err)
			os.Exit(1)
		}

		// load workload instance config
		configContent, err := ioutil.ReadFile(createWorkloadInstanceConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var workloadInstanceConfig config.WorkloadInstanceConfig
		if err := yaml.Unmarshal(configContent, &workloadInstanceConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}

		apiClient, err := clientInternal.GetHTTPClient(authEnabled)
		if err != nil {
			fmt.Errorf("failed to create https client: %w", err)
			os.Exit(1)
		}

		// create workload instance
		workloadInstance := workloadInstanceConfig.WorkloadInstance
		wi, err := workloadInstance.Create(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to create workload", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("workload instance %s created\n", *wi.Name))
	},
}

func init() {
	createCmd.AddCommand(CreateWorkloadInstanceCmd)

	CreateWorkloadInstanceCmd.Flags().StringVarP(
		&createWorkloadInstanceConfigPath,
		"config", "c", "", "Path to file with workload instance config.",
	)
	CreateWorkloadInstanceCmd.MarkFlagRequired("config")
}
