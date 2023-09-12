/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"os"

	"github.com/spf13/cobra"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	"github.com/threeport/threeport/pkg/threeport-installer/v0/tptdev"
)

// downCmd represents the down command
var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Spin down a threeport development environment",
	Long:  `Spin down a threeport development environment.`,
	Run: func(cmd *cobra.Command, args []string) {
		err := cliArgs.DeleteControlPlane(nil)
		if err != nil {
			cli.Error("failed to delete threeport control plane", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(downCmd)

	downCmd.Flags().StringVarP(&cliArgs.InstanceName,
		"name", "n", tptdev.DefaultInstanceName, "name of dev control plane instance")
	downCmd.Flags().StringVarP(&cliArgs.KubeconfigPath,
		"kubeconfig", "k", "", "path to kubeconfig - default is ~/.kube/config")
}
