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
	client_v0 "github.com/threeport/threeport/pkg/client/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// outputGetWorkloadsCmd produces the tabular output for the
// 'tptctl get workloads' command.
func outputGetWorkloadsCmd(
	v0workloadInstances *[]v0.WorkloadInstance,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
	fmt.Fprintln(writer, "VERSION\t NAME\t WORKLOAD DEFINITION\t WORKLOAD INSTANCE\t KUBERNETES RUNTIME INSTANCE\t STATUS\t AGE")
	metadataErr := false
	var workloadDefErr error
	var kubernetesRuntimeInstErr error
	var statusErr error
	for _, wi := range *v0workloadInstances {
		// get workload definition name for instance
		var workloadDef string
		workloadDefinition, err := client_v0.GetWorkloadDefinitionByID(
			apiClient,
			apiEndpoint,
			*wi.WorkloadDefinitionID,
		)
		if err != nil {
			metadataErr = true
			workloadDefErr = err
			workloadDef = "<error>"
		} else {
			workloadDef = *workloadDefinition.Name
		}
		// get kubernetes runtime instance name for instance
		var kubernetesRuntimeInst string
		kubernetesRuntimeInstance, err := client_v0.GetKubernetesRuntimeInstanceByID(
			apiClient,
			apiEndpoint,
			*wi.KubernetesRuntimeInstanceID,
		)
		if err != nil {
			metadataErr = true
			kubernetesRuntimeInstErr = err
			kubernetesRuntimeInst = "<error>"
		} else {
			kubernetesRuntimeInst = *kubernetesRuntimeInstance.Name
		}
		// get workload status
		var workloadInstStatus string
		workloadInstStatusDetail := status.GetWorkloadInstanceStatus(
			apiClient,
			apiEndpoint,
			agent.WorkloadInstanceType,
			*wi.ID,
			*wi.Reconciled,
		)
		if workloadInstStatusDetail.Error != nil {
			metadataErr = true
			statusErr = workloadInstStatusDetail.Error
			workloadInstStatus = "<error>"
		} else {
			workloadInstStatus = string(workloadInstStatusDetail.Status)
		}
		fmt.Fprintln(
			writer,
			"v0", "\t",
			workloadDef, "\t",
			workloadDef, "\t",
			*wi.Name, "\t",
			kubernetesRuntimeInst, "\t",
			workloadInstStatus, "\t",
			util.GetAge(wi.CreatedAt),
		)
	}
	writer.Flush()

	if metadataErr {
		multiError := util.MultiError{}
		if workloadDefErr != nil {
			multiError.AppendError(
				fmt.Errorf("encountered an error retrieving workload definition info: %w", workloadDefErr),
			)
		}
		if kubernetesRuntimeInstErr != nil {
			multiError.AppendError(
				fmt.Errorf("encountered an error retrieving kubernetes runtime instance info: %w", kubernetesRuntimeInstErr),
			)
		}
		if statusErr != nil {
			multiError.AppendError(
				fmt.Errorf("encountered an error retrieving workload instance status: %w", statusErr),
			)
		}
		return multiError.Error()
	}

	return nil
}

// outputGetv0WorkloadDefinitionsCmd produces the tabular output for the
// 'tptctl get workload-definitions' command.
func outputGetv0WorkloadDefinitionsCmd(
	workloadDefinitions *[]v0.WorkloadDefinition,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
	fmt.Fprintln(writer, "NAME\t AGE")
	for _, workloadDefinition := range *workloadDefinitions {
		fmt.Fprintln(
			writer,
			*workloadDefinition.Name, "\t",
			util.GetAge(workloadDefinition.CreatedAt),
		)
	}
	writer.Flush()

	return nil
}

// outputGetv0WorkloadInstancesCmd produces the tabular output for the
// 'tptctl get workload-instances' command.
func outputGetv0WorkloadInstancesCmd(
	workloadInstances *[]v0.WorkloadInstance,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
	fmt.Fprintln(writer, "NAME\t WORKLOAD DEFINITION\t KUBERNETES RUNTIME INSTANCE\t STATUS\t AGE")
	metadataErr := false
	var workloadDefErr error
	var kubernetesRuntimeInstErr error
	var statusErr error
	for _, wi := range *workloadInstances {
		// get workload definition name for instance
		var workloadDef string
		workloadDefinition, err := client_v0.GetWorkloadDefinitionByID(
			apiClient,
			apiEndpoint,
			*wi.WorkloadDefinitionID,
		)
		if err != nil {
			metadataErr = true
			workloadDefErr = err
			workloadDef = "<error>"
		} else {
			workloadDef = *workloadDefinition.Name
		}
		// get kubernetes runtime instance name for instance
		var kubernetesRuntimeInst string
		kubernetesRuntimeInstance, err := client_v0.GetKubernetesRuntimeInstanceByID(
			apiClient,
			apiEndpoint,
			*wi.KubernetesRuntimeInstanceID,
		)
		if err != nil {
			metadataErr = true
			kubernetesRuntimeInstErr = err
			kubernetesRuntimeInst = "<error>"
		} else {
			kubernetesRuntimeInst = *kubernetesRuntimeInstance.Name
		}
		// get workload status
		var workloadInstStatus string
		workloadInstStatusDetail := status.GetWorkloadInstanceStatus(
			apiClient,
			apiEndpoint,
			agent.WorkloadInstanceType,
			*wi.ID,
			*wi.Reconciled,
		)
		if workloadInstStatusDetail.Error != nil {
			metadataErr = true
			statusErr = workloadInstStatusDetail.Error
			workloadInstStatus = "<error>"
		} else {
			workloadInstStatus = string(workloadInstStatusDetail.Status)
		}

		fmt.Fprintln(
			writer,
			*wi.Name, "\t",
			workloadDef, "\t",
			kubernetesRuntimeInst, "\t",
			workloadInstStatus, "\t",
			util.GetAge(wi.CreatedAt),
		)
	}
	writer.Flush()

	if metadataErr {
		multiError := util.MultiError{}
		if workloadDefErr != nil {
			multiError.AppendError(
				fmt.Errorf("encountered an error retrieving workload definition info: %w", workloadDefErr),
			)
		}
		if kubernetesRuntimeInstErr != nil {
			multiError.AppendError(
				fmt.Errorf("encountered an error retrieving kubernetes runtime instance info: %w", kubernetesRuntimeInstErr),
			)
		}
		if statusErr != nil {
			multiError.AppendError(
				fmt.Errorf("encountered an error retrieving workload instance status: %w", statusErr),
			)
		}
		return multiError.Error()
	}

	return nil
}
