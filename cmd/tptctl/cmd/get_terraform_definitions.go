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

// GetTerraformDefinitionsCmd represents the terraform-definitions command
var GetTerraformDefinitionsCmd = &cobra.Command{
	Use:          "terraform-definitions",
	Example:      "tptctl get terraform-definitions",
	Short:        "Get terraform definitions from the system",
	Long:         `Get terraform definitions from the system.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, requestedControlPlane := getClientContext(cmd)

		// get terraform definitions
		terraformDefinitions, err := client.GetTerraformDefinitions(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to retrieve terraform definitions", err)
			os.Exit(1)
		}

		// write the output
		if len(*terraformDefinitions) == 0 {
			cli.Info(fmt.Sprintf(
				"No terraform definitions currently managed by %s threeport control plane",
				requestedControlPlane,
			))
			os.Exit(0)
		}
		writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
		fmt.Fprintln(writer, "NAME\t AGE")
		for _, wd := range *terraformDefinitions {
			fmt.Fprintln(writer, *wd.Name, "\t", util.GetAge(wd.CreatedAt))
		}
		writer.Flush()
	},
}

func init() {
	GetCmd.AddCommand(GetTerraformDefinitionsCmd)
	GetTerraformDefinitionsCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
