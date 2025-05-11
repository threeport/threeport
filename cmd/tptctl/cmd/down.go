/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"os"

	"github.com/spf13/cobra"
	cli "github.com/threeport/threeport/pkg/cli/v0"
)

// DownCmd represents the delete threeports
var DownCmd = &cobra.Command{
	Use:          "down",
	Example:      "tptctl down --name my-threeport",
	Short:        "Spin down a deployment of the Threeport control plane",
	Long:         `Spin down a deployment of the Threeport control plane.`,
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		cpi, err := cliArgs.CreateInstaller()
		if err != nil {
			cli.Error("failed to create threeport control plane installer", err)
			os.Exit(1)
		}

		err = cli.DeleteGenesisControlPlane(cpi)
		if err != nil {
			cli.Error("failed to delete threeport control plane", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(DownCmd)

	DownCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"name", "n", "", "Required. Name of genesis control plane.",
	)
	DownCmd.Flags().BoolVar(
		&cliArgs.ControlPlaneOnly,
		"control-plane-only", false, "Tear down the control plane and leave runtime intact. Defaults to false.",
	)
	DownCmd.Flags().BoolVar(
		&cliArgs.InfraOnly,
		"infra-only", false, "Tear down only the infrastructure without the control plane. Defaults to false.",
	)
	DownCmd.Flags().BoolVar(
		&cliArgs.AwsConfigEnv,
		"aws-config-env", false, "Retrieve AWS credentials from environment variables when using eks provider.",
	)
	DownCmd.MarkFlagRequired("name")
}
