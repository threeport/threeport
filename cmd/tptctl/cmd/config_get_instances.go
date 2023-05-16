/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/threeport/threeport/internal/cli"
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
		threeportConfig, err := config.GetThreeportConfig()
		if err != nil {
			cli.Error("failed to get threeport config", err)
			os.Exit(1)
		}

		// output table of results
		writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
		fmt.Fprintln(writer, "NAME\t PROVIDER")
		for _, instance := range threeportConfig.Instances {
			fmt.Fprintln(writer, instance.Name, "\t", instance.Provider)
		}
		writer.Flush()
	},
}

func init() {
	configCmd.AddCommand(ConfigGetInstancesCmd)
}
