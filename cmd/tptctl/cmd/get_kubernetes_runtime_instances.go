/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/threeport/threeport/internal/cli"
	"github.com/threeport/threeport/internal/util"
	client "github.com/threeport/threeport/pkg/client/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
)

// GetKubernetesRuntimeInstancesCmd represents the kubernetes-runtime-instances command
var GetKubernetesRuntimeInstancesCmd = &cobra.Command{
	Use:          "kubernetes-runtime-instances",
	Example:      "tptctl get kubernetes-runtime-instances",
	Short:        "Get kubernetes runtime instances from the system",
	Long:         `Get kubernetes runtime instances from the system.`,
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
			cli.Error("failed to create threeport API client", err)
			os.Exit(1)
		}

		// get kubernetes runtime instances
		kubernetesRuntimeInstances, err := client.GetKubernetesRuntimeInstances(apiClient, apiEndpoint)
		if err != nil {
			cli.Error("failed to retrieve kubernetes runtime instances", err)
			os.Exit(1)
		}

		// write the output
		if len(*kubernetesRuntimeInstances) == 0 {
			cli.Info(fmt.Sprintf(
				"No kubernetes runtime instances currently managed by %s threeport control plane",
				threeportConfig.CurrentInstance,
			))
			os.Exit(0)
		}
		writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
		fmt.Fprintln(writer, "NAME\t RUNTIME DEFINITION\t LOCATION\t DEFAULT RUNTIME\t INFRA PROVIDER\t AGE")
		metadataErr := false
		var kubernetesRuntimeDefErr error
		var kubernetesRuntimeInstErr error
		var statusErr error
		for _, kri := range *kubernetesRuntimeInstances {
			// get workload definition name for instance
			var kubernetesRuntimeDef string
			var infraProvider string
			kubernetesRuntimeDefinition, err := client.GetKubernetesRuntimeDefinitionByID(
				apiClient,
				apiEndpoint,
				*kri.KubernetesRuntimeDefinitionID,
			)
			if err != nil {
				metadataErr = true
				kubernetesRuntimeDefErr = err
				kubernetesRuntimeDef = "<error>"
				infraProvider = "<error>"
			} else {
				kubernetesRuntimeDef = *kubernetesRuntimeDefinition.Name
				infraProvider = *kubernetesRuntimeDefinition.InfraProvider
			}
			fmt.Println("$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$")
			fmt.Println(*kri.Location)
			fmt.Fprintln(
				writer, *kri.Name, "\t", kubernetesRuntimeDef, "\t", *kri.Location, "\t",
				*kri.DefaultRuntime, "\t", infraProvider, "\t", util.GetAge(kri.CreatedAt),
			)
		}
		writer.Flush()

		if metadataErr {
			if kubernetesRuntimeDefErr != nil {
				cli.Error("encountered an error retrieving kubernetes runtime definition info", kubernetesRuntimeDefErr)
			}
			if kubernetesRuntimeInstErr != nil {
				cli.Error("encountered an error retrieving kubernetes runtime instance info", kubernetesRuntimeInstErr)
			}
			if statusErr != nil {
				cli.Error("encountered an error retrieving kubernetes runtime instance status", statusErr)
			}
			os.Exit(1)
		}
	},
}

func init() {
	getCmd.AddCommand(GetKubernetesRuntimeInstancesCmd)
}
