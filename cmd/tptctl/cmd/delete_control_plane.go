/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/threeport/threeport/internal/cli"
	"github.com/threeport/threeport/internal/provider"
	"github.com/threeport/threeport/internal/threeport"
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
		// get threeport config
		threeportConfig := config.GetThreeportConfig()

		// check threeport config for exisiting instance
		// find the threeport instance by name
		threeportInstanceConfigExists := false
		var instanceConfig config.Instance
		for _, instance := range threeportConfig.Instances {
			if instance.Name == deleteThreeportInstanceName {
				instanceConfig = instance
				threeportInstanceConfigExists = true
			}
		}
		if !threeportInstanceConfigExists {
			cli.Error("failed to find threeport instance config",
				errors.New(fmt.Sprintf(
					"config for threeport instance with name %s not found", deleteThreeportInstanceName)))
			os.Exit(1)
		}

		var controlPlaneInfra provider.ControlPlaneInfra
		switch instanceConfig.Provider {
		case threeport.ControlPlaneInfraProviderKind:
			controlPlaneInfraKind := provider.ControlPlaneInfraKind{
				ThreeportInstanceName: instanceConfig.Name,
				KubeconfigPath:        instanceConfig.Kubeconfig,
			}
			controlPlaneInfra = &controlPlaneInfraKind
		}
		if err := controlPlaneInfra.Delete(); err != nil {
			cli.Error("failed to delete control plane infra", err)
			os.Exit(1)
		}

		config.DeleteThreeportConfigInstance(threeportConfig, deleteThreeportInstanceName)

		cli.Info("threeport config updated")

		cli.Complete(fmt.Sprintf("threeport instance %s deleted", deleteThreeportInstanceName))
	},
}

func init() {
	deleteCmd.AddCommand(DeleteControlPlaneCmd)

	DeleteControlPlaneCmd.Flags().StringVarP(
		&deleteThreeportInstanceName,
		"name", "n", "", "Required. Name of control plane instance.",
	)
	DeleteControlPlaneCmd.MarkFlagRequired("name")
}
