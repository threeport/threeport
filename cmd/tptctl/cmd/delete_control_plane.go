/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"os"

	"github.com/spf13/cobra"
	cli "github.com/threeport/threeport/pkg/cli/v0"
)

// DeleteControlPlaneCmd represents the delete control-plane command
var DeleteControlPlaneCmd = &cobra.Command{
	Use:          "control-plane",
	Example:      "tptctl delete control-plane --name my-threeport",
	Short:        "Delete an instance of the Threeport control plane",
	Long:         `Delete an instance of the Threeport control plane.`,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		err := cliArgs.DeleteControlPlane(nil)
		if err != nil {
			cli.Error("failed to delete threeport control plane", err)
			os.Exit(1)
		}
	},
}

func init() {
	deleteCmd.AddCommand(DeleteControlPlaneCmd)

	DeleteControlPlaneCmd.Flags().StringVarP(
		&cliArgs.InstanceName,
		"name", "n", "", "Required. Name of control plane instance.",
	)
	DeleteControlPlaneCmd.Flags().BoolVar(
		&cliArgs.AwsConfigEnv,
		"aws-config-env", false, "Retrieve AWS credentials from environment variables when using eks provider.",
	)
	DeleteControlPlaneCmd.MarkFlagRequired("name")
}
