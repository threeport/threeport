/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"

	"github.com/threeport/threeport/internal/cli"
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
		threeportConfig := &config.ThreeportConfig{}
		if err := viper.Unmarshal(threeportConfig); err != nil {
			cli.Error("Failed to get threeport config", err)
			os.Exit(1)
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

		apiClient, err := config.GetHTTPClient(authEnabled)
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
