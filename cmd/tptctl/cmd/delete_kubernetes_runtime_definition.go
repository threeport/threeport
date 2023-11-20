/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"net/http"
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
	Run: func(cmd *cobra.Command, args []string) {

		// get threeport config and extract threeport API endpoint
		threeportConfig, requestedControlPlane, err := config.GetThreeportConfig(cliArgs.ControlPlaneName)
		if err != nil {
			cli.Error("failed to get threeport config", err)
			os.Exit(1)
		}

		var apiClient *http.Client
		var apiEndpoint string

		apiClient, apiEndpoint = checkContext(cmd)
		if apiClient == nil && apiEndpoint != "" {
			apiEndpoint, err = threeportConfig.GetThreeportAPIEndpoint(requestedControlPlane)
			if err != nil {
				cli.Error("failed to get threeport API endpoint from config", err)
				os.Exit(1)
			}

			apiClient, err = threeportConfig.GetHTTPClient(requestedControlPlane)
			if err != nil {
				cli.Error("failed to create threeport API client", err)
				os.Exit(1)
			}
		}

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
