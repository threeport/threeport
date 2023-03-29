/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/threeport/threeport/internal/provider/kind"
	"github.com/threeport/threeport/internal/tptctl/output"
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
	RunE: func(cmd *cobra.Command, args []string) error {
		// get default kubeconfig if not provided
		if deleteKubeconfig == "" {
			dk, err := defaultKubeconfig()
			if err != nil {
				return fmt.Errorf("failed to get path to default kubeconfig: %w", err)
			}
			deleteKubeconfig = dk
		}

		// delete kind cluster
		if err := kind.DeleteKindCluster(
			kindClusterName(deleteThreeportDevName),
			deleteKubeconfig,
		); err != nil {
			return err
		}

		output.Complete(fmt.Sprintf("Threeport dev instance %s deleted", deleteThreeportDevName))

		return nil
	},
}

func init() {
	rootCmd.AddCommand(downCmd)

	downCmd.Flags().StringVarP(&deleteThreeportDevName,
		"name", "n", defaultDevName, "name of dev control plane instance")
	downCmd.Flags().StringVarP(&deleteKubeconfig,
		"kubeconfig", "k", "", "path to kubeconfig - default is ~/.kube/config")
}
