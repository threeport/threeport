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

// GetHelmWorkloadDefinitionsCmd represents the helm-workload-definitions command
var GetHelmWorkloadDefinitionsCmd = &cobra.Command{
	Use:          "helm-workload-definitions",
	Example:      "tptctl get helm-workload-definitions",
	Short:        "Get helm workload definitions from the system",
	Long:         `Get helm workload definitions from the system.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, requestedControlPlane := getClientContext(cmd)

		// get helm workload definitions
		helmWorkloadDefinitions, err := client.GetHelmWorkloadDefinitions(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to retrieve helm workload definitions", err)
			os.Exit(1)
		}

		// write the output
		if len(*helmWorkloadDefinitions) == 0 {
			cli.Info(fmt.Sprintf(
				"No helm workload definitions currently managed by %s threeport control plane",
				requestedControlPlane,
			))
			os.Exit(0)
		}
		writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
		fmt.Fprintln(writer, "NAME\t REPO\t CHART\t AGE")
		for _, wd := range *helmWorkloadDefinitions {
			fmt.Fprintln(writer, *wd.Name, "\t", *wd.Repo, "\t", *wd.Chart, "\t", util.GetAge(wd.CreatedAt))
		}
		writer.Flush()
	},
}

func init() {
	GetCmd.AddCommand(GetHelmWorkloadDefinitionsCmd)
	GetHelmWorkloadDefinitionsCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
