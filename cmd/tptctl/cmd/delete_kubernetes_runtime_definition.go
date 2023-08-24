/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
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
		threeportConfig, err := config.GetThreeportConfig()
		requestedInstance := threeportConfig.GetRequestedInstanceName(cliArgs.InstanceName)
		if err != nil {
			cli.Error("failed to get threeport config", err)
			os.Exit(1)
		}

		apiEndpoint, err := threeportConfig.GetThreeportAPIEndpoint(requestedInstance)
		if err != nil {
			cli.Error("failed to get threeport API endpoint from config", err)
			os.Exit(1)
		}

		kubernetesRuntimeDefinitionConfig := config.KubernetesRuntimeDefinitionConfig{
			KubernetesRuntimeDefinition: config.KubernetesRuntimeDefinitionValues{
				Name: deleteKubernetesRuntimeDefinitionName,
			},
		}

		// get threeport API client
		cliArgs.AuthEnabled, err = threeportConfig.GetThreeportAuthEnabled(requestedInstance)
		if err != nil {
			cli.Error("failed to determine if auth is enabled on threeport API", err)
			os.Exit(1)
		}
		ca, clientCertificate, clientPrivateKey, err := threeportConfig.GetThreeportCertificatesForInstance(requestedInstance)
		if err != nil {
			cli.Error("failed to get threeport certificates from config", err)
			os.Exit(1)
		}
		apiClient, err := client.GetHTTPClient(cliArgs.AuthEnabled, ca, clientCertificate, clientPrivateKey, "")
		if err != nil {
			cli.Error("failed to create https client", err)
			os.Exit(1)
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
	deleteCmd.AddCommand(DeleteKubernetesRuntimeDefinitionCmd)

	DeleteKubernetesRuntimeDefinitionCmd.Flags().StringVarP(
		&deleteKubernetesRuntimeDefinitionName,
		"name", "n", "", "Name of kubernetes runtime definition.",
	)
	DeleteKubernetesRuntimeDefinitionCmd.MarkFlagRequired("name")
}
