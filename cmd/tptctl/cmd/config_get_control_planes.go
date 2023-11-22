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

// ConfigGetControlPlanesCmd represents the get-instances command
var ConfigGetControlPlanesCmd = &cobra.Command{
	Use:          "get-control-planes",
	Example:      "tptctl config get-control-planes",
	Short:        "Get a list of threeport control planes in your threeport config",
	Long:         `Get a list of threeport control planes in your threeport config.`,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		// get threeport config
		threeportConfig, _, err := config.GetThreeportConfig(cliArgs.ControlPlaneName)
		if err != nil {
			cli.Error("failed to get threeport config", err)
			os.Exit(1)
		}

		// check to see if current control plane is set
		if threeportConfig.CurrentControlPlane == "" {
			cli.Warning("current control plane is not set - set it with 'tptctl config current-control-plane --control-plane-name <control-plane-name>'")
		}

		// output table of results
		writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
		fmt.Fprintln(writer, "NAME\t PROVIDER\t CURRENT CONTROL PLANE")
		for _, controlPlane := range threeportConfig.ControlPlanes {
			currentInst := false
			if controlPlane.Name == threeportConfig.CurrentControlPlane {
				currentInst = true
			}
			fmt.Fprintln(writer, controlPlane.Name, "\t", controlPlane.Provider, "\t", currentInst)
		}
		writer.Flush()
	},
}

func init() {
	ConfigCmd.AddCommand(ConfigGetControlPlanesCmd)
}
