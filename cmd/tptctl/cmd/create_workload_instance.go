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

	"github.com/threeport/threeport/internal/tptctl/config"
	"github.com/threeport/threeport/internal/tptctl/output"
)

var createWorkloadInstancePath string

// CreateWorkloadInstanceCmd represents the workload-instance command
var CreateWorkloadInstanceCmd = &cobra.Command{
	Use:          "workload-instance",
	Example:      "tptctl create workload-instance -c /path/to/config.yaml",
	Short:        "Create a new workload instance",
	Long:         `Create a new workload instance.`,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		// load config
		configContent, err := ioutil.ReadFile(createWorkloadInstancePath)
		if err != nil {
			output.Error("failed to read config file", err)
			os.Exit(1)
		}
		var workloadInstance config.WorkloadInstanceConfig
		if err := yaml.Unmarshal(configContent, &workloadInstance); err != nil {
			output.Error("failed to unmarshal config file yaml content", err)
			os.Exit(1)
		}

		// create workload instance
		wi, err := workloadInstance.Create()
		if err != nil {
			output.Error("failed to create workload", err)
			os.Exit(1)
		}

		output.Complete(fmt.Sprintf("workload instance %s created\n", *wi.Name))
	},
}

func init() {
	createCmd.AddCommand(CreateWorkloadInstanceCmd)

	CreateWorkloadInstanceCmd.Flags().StringVarP(&createWorkloadInstancePath, "config", "c", "", "path to file with workload instance config")
	CreateWorkloadInstanceCmd.MarkFlagRequired("config")
}
