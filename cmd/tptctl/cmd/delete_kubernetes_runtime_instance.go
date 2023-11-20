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

var deleteKubernetesRuntimeInstanceName string

// DeleteKubernetesRuntimeInstanceCmd represents the kubernetes-runtime-instance command
var DeleteKubernetesRuntimeInstanceCmd = &cobra.Command{
	Use:          "kubernetes-runtime-instance",
	Example:      "tptctl delete kubernetes-runtime-instance --name some-instance",
	Short:        "Delete an existing kubernetes runtime instance",
	Long:         `Delete an existing kubernetes runtime instance.`,
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

		kubernetesRuntimeInstanceConfig := config.KubernetesRuntimeInstanceConfig{
			KubernetesRuntimeInstance: config.KubernetesRuntimeInstanceValues{
				Name: deleteKubernetesRuntimeInstanceName,
			},
		}

		// delete kubernetes runtime instance
		kubernetesRuntimeInstance := kubernetesRuntimeInstanceConfig.KubernetesRuntimeInstance
		aa, err := kubernetesRuntimeInstance.Delete(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to delete kubernetes runtime instance", err)
			os.Exit(1)
		}

		cli.Complete(fmt.Sprintf("kubernetes runtime instance %s deleted", *aa.Name))
	},
}

func init() {
	DeleteCmd.AddCommand(DeleteKubernetesRuntimeInstanceCmd)

	DeleteKubernetesRuntimeInstanceCmd.Flags().StringVarP(
		&deleteKubernetesRuntimeInstanceName,
		"name", "n", "", "Name of kubernetes runtime instance.",
	)
	DeleteKubernetesRuntimeInstanceCmd.MarkFlagRequired("name")
	DeleteKubernetesRuntimeInstanceCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}
