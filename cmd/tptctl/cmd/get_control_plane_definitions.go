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
	client "github.com/threeport/threeport/pkg/client/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// GetControlPlaneDefinitionsCmd represents the control-plane-definitions command
var GetControlPlaneDefinitionsCmd = &cobra.Command{
	Use:          "control-plane-definitions",
	Example:      "tptctl get control-plane-definitions",
	Short:        "Get control plane definitions from the system",
	Long:         `Get control plane definitions from the system.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, threeportConfig, apiEndpoint, _ := getClientContext(cmd)

		// get control plane definitions
		controlPlaneDefinitions, err := client.GetControlPlaneDefinitions(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to retrieve control plane definitions", err)
			os.Exit(1)
		}

		// write the output
		if len(*controlPlaneDefinitions) == 0 {
			cli.Info(fmt.Sprintf(
				"No control plane definitions currently managed by %s threeport control plane",
				threeportConfig.CurrentControlPlane,
			))
			os.Exit(0)
		}
		writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
		fmt.Fprintln(writer, "NAME\t AGE")
		for _, wd := range *controlPlaneDefinitions {
			fmt.Fprintln(writer, *wd.Name, "\t", util.GetAge(wd.CreatedAt))
		}
		writer.Flush()
	},
}

func init() {
	GetCmd.AddCommand(GetControlPlaneDefinitionsCmd)
}
