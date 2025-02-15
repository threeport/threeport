// originally generated by 'threeport-sdk codegen api-model' but will not be regenerated - intended for modification

package cmd

import (
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// outputGetKubernetesRuntimesCmd produces the tabular output for the
// 'tptctl get kubernetes-runtimes' command.
func outputGetKubernetesRuntimesCmd(
	kubernetesRuntimeInstances *[]v0.KubernetesRuntimeInstance,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
	//fmt.Fprintln(writer, "NAME\t AGE")
	fmt.Fprintln(writer, "NAME\t KUBERNETES RUNTIME DEFINITION\t KUBERNETES RUNTIME INSTANCE\t AGE")
	var kubernetesRuntimeDefErr error
	for _, kubernetesRuntimeInstance := range *kubernetesRuntimeInstances {
		// get kubernetes runtime definition name for instance
		var kubernetesRuntimeDef string
		kubernetesRuntimeDefinition, err := client.GetKubernetesRuntimeDefinitionByID(
			apiClient,
			apiEndpoint,
			*kubernetesRuntimeInstance.KubernetesRuntimeDefinitionID,
		)
		if err != nil {
			kubernetesRuntimeDefErr = err
			kubernetesRuntimeDef = "<error>"
		} else {
			kubernetesRuntimeDef = *kubernetesRuntimeDefinition.Name
		}

		fmt.Fprintln(
			writer,
			*kubernetesRuntimeInstance.Name, "\t",
			kubernetesRuntimeDef, "\t",
			*kubernetesRuntimeInstance.Name, "\t",
			util.GetAge(kubernetesRuntimeInstance.CreatedAt),
		)
	}
	writer.Flush()

	if kubernetesRuntimeDefErr != nil {
		return fmt.Errorf("encountered an error retrieving kubernetes runtime definition info: %w", kubernetesRuntimeDefErr)
	}

	return nil
}

// outputGetv0KubernetesRuntimeDefinitionsCmd produces the tabular output for the
// 'tptctl get kubernetes-runtime-definitions' command.
func outputGetv0KubernetesRuntimeDefinitionsCmd(
	kubernetesRuntimeDefinitions *[]v0.KubernetesRuntimeDefinition,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
	fmt.Fprintln(writer, "NAME\t INFRA PROVIDER\t HIGH AVAILABILITY\t INFRA PROVIDER ACCOUNT\t AGE")
	for _, k := range *kubernetesRuntimeDefinitions {
		var ha bool
		if k.HighAvailability == nil {
			ha = false
		} else {
			ha = *k.HighAvailability
		}
		var providerAccountID string
		if k.InfraProviderAccountName == nil {
			providerAccountID = "N/A"
		} else {
			providerAccountID = *k.InfraProviderAccountName
		}
		fmt.Fprintln(
			writer,
			*k.Name, "\t",
			*k.InfraProvider, "\t",
			ha, "\t",
			providerAccountID, "\t",
			util.GetAge(k.CreatedAt),
		)
	}
	writer.Flush()

	return nil
}

// outputGetv0KubernetesRuntimeInstancesCmd produces the tabular output for the
// 'tptctl get kubernetes-runtime-instances' command.
func outputGetv0KubernetesRuntimeInstancesCmd(
	kubernetesRuntimeInstances *[]v0.KubernetesRuntimeInstance,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
	fmt.Fprintln(writer, "NAME\t KUBERNETES RUNTIME DEFINITION\t LOCATION\t DEFAULT RUNTIME\t INFRA PROVIDER\t FORCE DELETE\t AGE")
	var kubernetesRuntimeDefErr error
	for _, k := range *kubernetesRuntimeInstances {
		// get workload definition name for instance
		var kubernetesRuntimeDef string
		var infraProvider string
		kubernetesRuntimeDefinition, err := client.GetKubernetesRuntimeDefinitionByID(
			apiClient,
			apiEndpoint,
			*k.KubernetesRuntimeDefinitionID,
		)
		if err != nil {
			kubernetesRuntimeDefErr = err
			kubernetesRuntimeDef = "<error>"
			infraProvider = "<error>"
		} else {
			kubernetesRuntimeDef = *kubernetesRuntimeDefinition.Name
			infraProvider = *kubernetesRuntimeDefinition.InfraProvider
		}
		fmt.Fprintln(
			writer,
			*k.Name, "\t",
			kubernetesRuntimeDef, "\t",
			*k.Location, "\t",
			*k.DefaultRuntime, "\t",
			infraProvider, "\t",
			*k.ForceDelete, "\t",
			util.GetAge(k.CreatedAt),
		)
	}
	writer.Flush()

	if kubernetesRuntimeDefErr != nil {
		return fmt.Errorf("encountered an error retrieving kubernetes runtime definition info: %w", kubernetesRuntimeDefErr)
	}

	return nil
}
