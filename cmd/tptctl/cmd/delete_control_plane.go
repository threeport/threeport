/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/threeport/threeport/internal/cli"
)

// DeleteControlPlaneCmd represents the delete control-plane command
var DeleteControlPlaneCmd = &cobra.Command{
	Use:          "control-plane",
	Example:      "tptctl delete control-plane --name my-threeport",
	Short:        "Delete an instance of the Threeport control plane",
	Long:         `Delete an instance of the Threeport control plane.`,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		err := cliArgs.DeleteControlPlane()
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
	DeleteControlPlaneCmd.Flags().StringVar(
		&cliArgs.AwsConfigProfile,
		"aws-config-profile", "default", "The AWS config profile to draw credentials from when using eks provider.",
	)
	DeleteControlPlaneCmd.Flags().StringVar(
		&cliArgs.AwsRegion,
		"aws-region", "", "AWS region code to install threeport in when using eks provider. If provided, will take precedence over AWS config profile and environment variables.",
	)
	DeleteControlPlaneCmd.MarkFlagRequired("name")
}
