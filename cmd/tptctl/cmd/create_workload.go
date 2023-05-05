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

var createWorkloadConfigPath string

// CreateWorkloadCmd represents the workload command
var CreateWorkloadCmd = &cobra.Command{
	Use:     "workload",
	Example: "tptctl create workload --config /path/to/config.yaml",
	Short:   "Create a new workload",
	Long: `Create a new workload. This command creates a new workload definition
and workload instance based on the workload config.`,
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

		// load workload config
		configContent, err := ioutil.ReadFile(createWorkloadConfigPath)
		if err != nil {
			cli.Error("failed to read config file", err)
			os.Exit(1)
		}
		var workloadConfig config.WorkloadConfig
		if err := yaml.Unmarshal(configContent, &workloadConfig); err != nil {
			cli.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}

		apiClient, err := clientInternal.GetHTTPClient(authEnabled)
		if err != nil {
			fmt.Errorf("failed to create https client: %w", err)
			os.Exit(1)
		}

		// create workload
		workload := workloadConfig.Workload
		wd, wi, err := workload.Create(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to create workload", err)
			os.Exit(1)
		}

		cli.Info(fmt.Sprintf("workload definition %s created", *wd.Name))
		cli.Info(fmt.Sprintf("workload instance %s created", *wi.Name))
		cli.Complete(fmt.Sprintf("workload %s created", workloadConfig.Workload.Name))
	},
}

func init() {
	createCmd.AddCommand(CreateWorkloadCmd)

	CreateWorkloadCmd.Flags().StringVarP(
		&createWorkloadConfigPath,
		"config", "c", "", "Path to file with workload config.",
	)
	CreateWorkloadCmd.MarkFlagRequired("config")
}
