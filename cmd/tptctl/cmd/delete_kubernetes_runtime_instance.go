/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/threeport/threeport/internal/cli"
	client "github.com/threeport/threeport/pkg/client/v0"
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
		threeportConfig, err := config.GetThreeportConfig()
		if err != nil {
			cli.Error("failed to get threeport config", err)
			os.Exit(1)
		}

		apiEndpoint, err := threeportConfig.GetThreeportAPIEndpoint()
		if err != nil {
			cli.Error("failed to get threeport API endpoint from config", err)
			os.Exit(1)
		}

		kubernetesRuntimeInstanceConfig := config.KubernetesRuntimeInstanceConfig{
			KubernetesRuntimeInstance: config.KubernetesRuntimeInstanceValues{
				Name: deleteKubernetesRuntimeInstanceName,
			},
		}

		// get threeport API client
		cliArgs.AuthEnabled, err = threeportConfig.GetThreeportAuthEnabled()
		if err != nil {
			cli.Error("failed to determine if auth is enabled on threeport API", err)
			os.Exit(1)
		}
		ca, clientCertificate, clientPrivateKey, err := threeportConfig.GetThreeportCertificates()
		if err != nil {
			cli.Error("failed to get threeport certificates from config", err)
			os.Exit(1)
		}
		apiClient, err := client.GetHTTPClient(cliArgs.AuthEnabled, ca, clientCertificate, clientPrivateKey)
		if err != nil {
			cli.Error("failed to create https client", err)
			os.Exit(1)
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
	deleteCmd.AddCommand(DeleteKubernetesRuntimeInstanceCmd)

	DeleteKubernetesRuntimeInstanceCmd.Flags().StringVarP(
		&deleteKubernetesRuntimeInstanceName,
		"name", "n", "", "Name of kubernetes runtime instance.",
	)
	DeleteKubernetesRuntimeInstanceCmd.MarkFlagRequired("name")
}
