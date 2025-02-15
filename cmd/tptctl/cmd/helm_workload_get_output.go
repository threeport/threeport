// originally generated by 'threeport-sdk codegen api-model' but will not be regenerated - intended for modification

package cmd

import (
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"

	"github.com/threeport/threeport/internal/agent"
	"github.com/threeport/threeport/internal/workload/status"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// outputGetHelmWorkloadsCmd produces the tabular output for the
// 'tptctl get helm-workloads' command.
func outputGetHelmWorkloadsCmd(
	helmWorkloadInstances *[]v0.HelmWorkloadInstance,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
	fmt.Fprintln(writer, "NAME\t HELM WORKLOAD DEFINITION\t HELM WORKLOAD INSTANCE\t KUBERNETES RUNTIME INSTANCE\t STATUS\t AGE")
	metadataErr := false
	var helmWorkloadDefErr error
	var kubernetesRuntimeInstErr error
	var statusErr error
	for _, h := range *helmWorkloadInstances {
		// get helm workload definition name for instance
		var helmWorkloadDef string
		helmWorkloadDefinition, err := client.GetHelmWorkloadDefinitionByID(apiClient, apiEndpoint, *h.HelmWorkloadDefinitionID)
		if err != nil {
			metadataErr = true
			helmWorkloadDefErr = err
			helmWorkloadDef = "<error>"
		} else {
			helmWorkloadDef = *helmWorkloadDefinition.Name
		}
		// get kubernetes runtime instance name for instance
		var kubernetesRuntimeInst string
		kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByID(apiClient, apiEndpoint, *h.KubernetesRuntimeInstanceID)
		if err != nil {
			metadataErr = true
			kubernetesRuntimeInstErr = err
			kubernetesRuntimeInst = "<error>"
		} else {
			kubernetesRuntimeInst = *kubernetesRuntimeInstance.Name
		}
		// get helm workload status
		var helmWorkloadInstStatus string
		helmWorkloadInstStatusDetail := status.GetWorkloadInstanceStatus(
			apiClient,
			apiEndpoint,
			agent.HelmWorkloadInstanceType,
			*h.ID,
			*h.Reconciled,
		)
		if helmWorkloadInstStatusDetail.Error != nil {
			metadataErr = true
			statusErr = helmWorkloadInstStatusDetail.Error
			helmWorkloadInstStatus = "<error>"
		} else {
			helmWorkloadInstStatus = string(helmWorkloadInstStatusDetail.Status)
		}

		fmt.Fprintln(
			writer,
			helmWorkloadDef, "\t",
			helmWorkloadDef, "\t",
			*h.Name, "\t",
			kubernetesRuntimeInst, "\t",
			helmWorkloadInstStatus, "\t",
			util.GetAge(h.CreatedAt),
		)
	}
	writer.Flush()

	if metadataErr {
		multiError := util.MultiError{}
		if helmWorkloadDefErr != nil {
			multiError.AppendError(
				fmt.Errorf("encountered an error retrieving helm workload definition info: %w", helmWorkloadDefErr),
			)
		}
		if kubernetesRuntimeInstErr != nil {
			multiError.AppendError(
				fmt.Errorf("encountered an error retrieving kubernetes runtime instance info: %w", kubernetesRuntimeInstErr),
			)
		}
		if statusErr != nil {
			multiError.AppendError(
				fmt.Errorf("encountered an error retrieving helm workload instance status: %w", statusErr),
			)
		}
		return multiError.Error()
	}

	return nil
}

// outputGetv0HelmWorkloadDefinitionsCmd produces the tabular output for the
// 'tptctl get helm-workload-definitions' command.
func outputGetv0HelmWorkloadDefinitionsCmd(
	helmWorkloadDefinitions *[]v0.HelmWorkloadDefinition,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
	fmt.Fprintln(writer, "NAME\t REPO\t CHART\t AGE")
	for _, h := range *helmWorkloadDefinitions {
		fmt.Fprintln(
			writer,
			*h.Name, "\t",
			*h.Repo, "\t",
			*h.Chart, "\t",
			util.GetAge(h.CreatedAt),
		)
	}
	writer.Flush()

	return nil
}

// outputGetv0HelmWorkloadInstancesCmd produces the tabular output for the
// 'tptctl get helm-workload-instances' command.
func outputGetv0HelmWorkloadInstancesCmd(
	helmWorkloadInstances *[]v0.HelmWorkloadInstance,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
	fmt.Fprintln(writer, "NAME\t HELM WORKLOAD DEFINITION\t KUBERNETES RUNTIME INSTANCE\t STATUS\t AGE")
	metadataErr := false
	var helmWorkloadDefErr error
	var kubernetesRuntimeInstErr error
	var statusErr error
	for _, h := range *helmWorkloadInstances {
		// get helm workload definition name for instance
		var helmWorkloadDef string
		helmWorkloadDefinition, err := client.GetHelmWorkloadDefinitionByID(apiClient, apiEndpoint, *h.HelmWorkloadDefinitionID)
		if err != nil {
			metadataErr = true
			helmWorkloadDefErr = err
			helmWorkloadDef = "<error>"
		} else {
			helmWorkloadDef = *helmWorkloadDefinition.Name
		}
		// get kubernetes runtime instance name for instance
		var kubernetesRuntimeInst string
		kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByID(apiClient, apiEndpoint, *h.KubernetesRuntimeInstanceID)
		if err != nil {
			metadataErr = true
			kubernetesRuntimeInstErr = err
			kubernetesRuntimeInst = "<error>"
		} else {
			kubernetesRuntimeInst = *kubernetesRuntimeInstance.Name
		}
		// get helm workload status
		var helmWorkloadInstStatus string
		helmWorkloadInstStatusDetail := status.GetWorkloadInstanceStatus(
			apiClient,
			apiEndpoint,
			agent.HelmWorkloadInstanceType,
			*h.ID,
			*h.Reconciled,
		)
		if helmWorkloadInstStatusDetail.Error != nil {
			metadataErr = true
			statusErr = helmWorkloadInstStatusDetail.Error
			helmWorkloadInstStatus = "<error>"
		} else {
			helmWorkloadInstStatus = string(helmWorkloadInstStatusDetail.Status)
		}

		fmt.Fprintln(
			writer,
			*h.Name, "\t",
			helmWorkloadDef, "\t",
			kubernetesRuntimeInst, "\t",
			helmWorkloadInstStatus, "\t",
			util.GetAge(h.CreatedAt),
		)
	}
	writer.Flush()

	if metadataErr {
		multiError := util.MultiError{}
		if helmWorkloadDefErr != nil {
			multiError.AppendError(
				fmt.Errorf("encountered an error retrieving helm workload definition info: %w", helmWorkloadDefErr),
			)
		}
		if kubernetesRuntimeInstErr != nil {
			multiError.AppendError(
				fmt.Errorf("encountered an error retrieving kubernetes runtime instance info: %w", kubernetesRuntimeInstErr),
			)
		}
		if statusErr != nil {
			multiError.AppendError(
				fmt.Errorf("encountered an error retrieving helm workload instance status: %w", statusErr),
			)
		}
		return multiError.Error()
	}

	return nil
}
