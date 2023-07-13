/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/threeport/threeport/internal/cli"
	"github.com/threeport/threeport/internal/tptdev"
)

// downCmd represents the down command
var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Spin down a threeport development environment",
	Long:  `Spin down a threeport development environment.`,
	Run: func(cmd *cobra.Command, args []string) {
		err := cliArgs.DeleteControlPlane()
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
