/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/threeport/threeport/internal/cli"
	internalCmd "github.com/threeport/threeport/internal/cmd"
	config "github.com/threeport/threeport/pkg/config/v0"
)

var deleteThreeportInstanceName string

// DeleteControlPlaneCmd represents the delete control-plane command
var DeleteControlPlaneCmd = &cobra.Command{
	Use:          "control-plane",
	Example:      "tptctl delete control-plane --name my-threeport",
	Short:        "Delete an instance of the Threeport control plane",
	Long:         `Delete an instance of the Threeport control plane.`,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		err := internalCmd.CreateControlPlane(cliArgs)
		if err != nil {
			cli.Error("failed to create threeport control plane", err)
			os.Exit(1)
		}
	},
}

func init() {
	deleteCmd.AddCommand(DeleteControlPlaneCmd)

	cliArgs = &config.CLIArgs{}

	DeleteControlPlaneCmd.Flags().StringVarP(
		&cliArgs.InstanceName,
		"name", "n", "", "Required. Name of control plane instance.",
	)
	DeleteControlPlaneCmd.MarkFlagRequired("name")
}
