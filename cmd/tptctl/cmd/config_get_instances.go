/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
)

// ConfigGetInstancesCmd represents the get-instances command
var ConfigGetInstancesCmd = &cobra.Command{
	Use:          "get-instances",
	Example:      "tptctl config get-instances",
	Short:        "Get a list of threeport instances in your threeport config",
	Long:         `Get a list of threeport instances in your threeport config.`,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		// get threeport config
		threeportConfig, _, err := config.GetThreeportConfig(cliArgs.InstanceName)
		if err != nil {
			cli.Error("failed to get threeport config", err)
			os.Exit(1)
		}

		// check to see if current instance is set
		if threeportConfig.CurrentInstance == "" {
			cli.Warning("current instance is not set - set it with 'tptctl config current-instance -i <instance name>'")
		}

		// output table of results
		writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
		fmt.Fprintln(writer, "NAME\t PROVIDER\t CURRENT INSTANCE")
		for _, instance := range threeportConfig.Instances {
			currentInst := false
			if instance.Name == threeportConfig.CurrentInstance {
				currentInst = true
			}
			fmt.Fprintln(writer, instance.Name, "\t", instance.Provider, "\t", currentInst)
		}
		writer.Flush()
	},
}

func init() {
	configCmd.AddCommand(ConfigGetInstancesCmd)
}
