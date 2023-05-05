/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/threeport/threeport/internal/cli"
	configInternal "github.com/threeport/threeport/internal/config"
	"github.com/threeport/threeport/internal/kube"
	"github.com/threeport/threeport/internal/provider"
	"github.com/threeport/threeport/internal/tptdev"
)

var (
	deleteThreeportDevName string
	deleteKubeconfig       string
)

// downCmd represents the down command
var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Spin down a threeport development environment",
	Long:  `Spin down a threeport development environment.`,
	Run: func(cmd *cobra.Command, args []string) {
		// get default kubeconfig if not provided
		if deleteKubeconfig == "" {
			dk, err := kube.DefaultKubeconfig()
			if err != nil {
				cli.Error("failed to get path to default kubeconfig", err)
				os.Exit(1)
			}
			deleteKubeconfig = dk
		}

		// delete kind cluster
		controlPlaneInfra := provider.ControlPlaneInfraKind{
			ThreeportInstanceName: deleteThreeportDevName,
			KubeconfigPath:        deleteKubeconfig,
		}
		if err := controlPlaneInfra.Delete(); err != nil {
			cli.Error("failed to delete control plane infra", err)
		}

		threeportConfig, err := configInternal.GetThreeportConfig()
		if err != nil {
			cli.Error("failed to get threeport config", err)
		}

		configInternal.DeleteThreeportConfigInstance(threeportConfig, deleteThreeportDevName)
		cli.Complete(fmt.Sprintf("threeport dev instance %s deleted", deleteThreeportDevName))
	},
}

func init() {
	rootCmd.AddCommand(downCmd)

	downCmd.Flags().StringVarP(&deleteThreeportDevName,
		"name", "n", tptdev.DefaultInstanceName, "name of dev control plane instance")
	downCmd.Flags().StringVarP(&deleteKubeconfig,
		"kubeconfig", "k", "", "path to kubeconfig - default is ~/.kube/config")
}
