/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
)

var deleteKubernetesRuntimeDefinitionName string

// DeleteKubernetesRuntimeDefinitionCmd represents the kubernetes-runtime-definition command
var DeleteKubernetesRuntimeDefinitionCmd = &cobra.Command{
	Use:          "kubernetes-runtime-definition",
	Example:      "tptctl delete kubernetes-runtime-definition --name some-definition",
	Short:        "Delete an existing kubernetes runtime definition",
	Long:         `Delete an existing kubernetes runtime definition.`,
	SilenceUsage: true,
	PreRun:       commandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, _ := getClientContext(cmd)

		kubernetesRuntimeDefinitionConfig := config.KubernetesRuntimeDefinitionConfig{
			KubernetesRuntimeDefinition: config.KubernetesRuntimeDefinitionValues{
				Name: deleteKubernetesRuntimeDefinitionName,
			},
		}

		// delete kubernetes runtime definition
		kubernetesRuntimeDefinition := kubernetesRuntimeDefinitionConfig.KubernetesRuntimeDefinition
		aa, err := kubernetesRuntimeDefinition.Delete(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to delete kubernetes runtime definition", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("kubernetes runtime definition %s deleted", *aa.Name))
	},
}

func init() {
	DeleteCmd.AddCommand(DeleteKubernetesRuntimeDefinitionCmd)

	DeleteKubernetesRuntimeDefinitionCmd.Flags().StringVarP(
		&deleteKubernetesRuntimeDefinitionName,
		"name", "n", "", "Name of kubernetes runtime definition.",
	)
	DeleteKubernetesRuntimeDefinitionCmd.MarkFlagRequired("name")
	DeleteKubernetesRuntimeDefinitionCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
