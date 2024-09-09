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

// outputGetObservabilityStacksCmd produces the tabular output for the
// 'tptctl get observability-stacks' command.
func outputGetObservabilityStacksCmd(
	observabilityStackInstances *[]v0.ObservabilityStackInstance,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
	fmt.Fprintln(writer, "NAME\t OBSERVABILITY STACK DEFINITION\t OBSERVABILITY STACK INSTANCE \t  AGE")
	var observabilityStackDefErr error
	for _, o := range *observabilityStackInstances {
		// get observability stack definition name for instance
		var observabilityStackDef string
		observabilityStackDefinition, err := client.GetObservabilityStackDefinitionByID(
			apiClient,
			apiEndpoint,
			*o.ObservabilityStackDefinitionID,
		)
		if err != nil {
			observabilityStackDefErr = err
			observabilityStackDef = "<error>"
		} else {
			observabilityStackDef = *observabilityStackDefinition.Name
		}

		fmt.Fprintln(
			writer,
			*o.Name, "\t",
			observabilityStackDef, "\t",
			*o.Name, "\t",
			util.GetAge(o.CreatedAt),
		)
	}
	writer.Flush()

	if observabilityStackDefErr != nil {
		return fmt.Errorf("encountered an error retrieving observability stack definition info: %w", observabilityStackDefErr)
	}

	return nil
}

// outputGetv0ObservabilityStackDefinitionsCmd produces the tabular output for the
// 'tptctl get observability-stack-definitions' command.
func outputGetv0ObservabilityStackDefinitionsCmd(
	observabilityStackDefinitions *[]v0.ObservabilityStackDefinition,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
	fmt.Fprintln(writer, "NAME\t OBSERVABILITY DASHBOARD DEFINITION\t METRICS DEFINITION\t LOGGING DEFINITION\t AGE")
	metadataErr := false
	var observabilityDashboardDefErr error
	var metricsDefErr error
	var loggingDefErr error
	for _, o := range *observabilityStackDefinitions {
		// get observability dashboard definition name for stack definition
		var observabilityDashboardDef string
		observabilityDashboardDefinition, err := client.GetObservabilityDashboardDefinitionByID(
			apiClient,
			apiEndpoint,
			*o.ObservabilityDashboardDefinitionID,
		)
		if err != nil {
			metadataErr = true
			observabilityDashboardDefErr = err
			observabilityDashboardDef = "<error>"
		} else {
			observabilityDashboardDef = *observabilityDashboardDefinition.Name
		}
		// get  definition name for stack definition
		var metricsDef string
		metricsDefinition, err := client.GetMetricsDefinitionByID(
			apiClient,
			apiEndpoint,
			*o.MetricsDefinitionID,
		)
		if err != nil {
			metadataErr = true
			metricsDefErr = err
			metricsDef = "<error>"
		} else {
			metricsDef = *metricsDefinition.Name
		}
		// get logging definition name for stack definition
		var loggingDef string
		loggingDefinition, err := client.GetLoggingDefinitionByID(
			apiClient,
			apiEndpoint,
			*o.LoggingDefinitionID,
		)
		if err != nil {
			metadataErr = true
			loggingDefErr = err
			loggingDef = "<error>"
		} else {
			loggingDef = *loggingDefinition.Name
		}

		fmt.Fprintln(
			writer,
			*o.Name, "\t",
			observabilityDashboardDef, "\t",
			metricsDef, "\t",
			loggingDef, "\t",
			util.GetAge(o.CreatedAt),
		)
	}
	writer.Flush()

	if metadataErr {
		multiError := util.MultiError{}
		if observabilityDashboardDefErr != nil {
			multiError.AppendError(
				fmt.Errorf("encountered an error retrieving observability dashboard definition info: %w", observabilityDashboardDefErr),
			)
		}
		if metricsDefErr != nil {
			multiError.AppendError(
				fmt.Errorf("encountered an error retrieving metrics definition info: %w", metricsDefErr),
			)
		}
		if loggingDefErr != nil {
			multiError.AppendError(
				fmt.Errorf("encountered an error retrieving logging definition info: %w", loggingDefErr),
			)
		}
	}

	return nil
}

// outputGetv0ObservabilityStackInstancesCmd produces the tabular output for the
// 'tptctl get observability-stack-instances' command.
func outputGetv0ObservabilityStackInstancesCmd(
	observabilityStackInstances *[]v0.ObservabilityStackInstance,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
	fmt.Fprintln(writer, "NAME\t OBSERVABILITY STACK DEFINITION\t KUBERNETES RUNTIME INSTANCE\t METRICS ENABLED\t LOGGING ENABLED\t OBSERVABILITY DASHBOARD INSTANCE\t MERTICS INSTANCE\t LOGGING INSTANCE\t AGE")
	metadataErr := false
	var observabilityStackDefErr error
	var kubernetesRuntimeInstErr error
	var observabilityDashboardInstErr error
	var metricsInstErr error
	var loggingInstErr error
	for _, o := range *observabilityStackInstances {
		// get observability stack definition name for instance
		var observabilityStackDef string
		observabilityStackDefinition, err := client.GetObservabilityStackDefinitionByID(
			apiClient,
			apiEndpoint,
			*o.ObservabilityStackDefinitionID,
		)
		if err != nil {
			metadataErr = true
			observabilityStackDefErr = err
			observabilityStackDef = "<error>"
		} else {
			observabilityStackDef = *observabilityStackDefinition.Name
		}
		// get kubernetes runtime instance name for instance
		var kubernetesRuntimeInst string
		kubernetesRuntimeInstance, err := client.GetKubernetesRuntimeInstanceByID(
			apiClient,
			apiEndpoint,
			*o.KubernetesRuntimeInstanceID,
		)
		if err != nil {
			metadataErr = true
			kubernetesRuntimeInstErr = err
			kubernetesRuntimeInst = "<error>"
		} else {
			kubernetesRuntimeInst = *kubernetesRuntimeInstance.Name
		}
		// get observability dashboard instance name for instance
		var observabilityDashboardInst string
		observabilityDashboardInstance, err := client.GetObservabilityDashboardInstanceByID(
			apiClient,
			apiEndpoint,
			*o.ObservabilityDashboardInstanceID,
		)
		if err != nil {
			metadataErr = true
			observabilityDashboardInstErr = err
			observabilityDashboardInst = "<error>"
		} else {
			observabilityDashboardInst = *observabilityDashboardInstance.Name
		}
		// get metrics instance name for instance
		var metricsInst string
		metricsInstance, err := client.GetMetricsInstanceByID(
			apiClient,
			apiEndpoint,
			*o.MetricsInstanceID,
		)
		if err != nil {
			metadataErr = true
			metricsInstErr = err
			metricsInst = "<error>"
		} else {
			metricsInst = *metricsInstance.Name
		}
		// get logging instance name for instance
		var loggingInst string
		loggingInstance, err := client.GetLoggingInstanceByID(
			apiClient,
			apiEndpoint,
			*o.LoggingInstanceID,
		)
		if err != nil {
			metadataErr = true
			loggingInstErr = err
			loggingInst = "<error>"
		} else {
			loggingInst = *loggingInstance.Name
		}

		fmt.Fprintln(
			writer,
			*o.Name, "\t",
			observabilityStackDef, "\t",
			kubernetesRuntimeInst, "\t",
			*o.MetricsEnabled, "\t",
			*o.LoggingEnabled, "\t",
			observabilityDashboardInst, "\t",
			metricsInst, "\t",
			loggingInst, "\t",
			util.GetAge(o.CreatedAt),
		)
	}
	writer.Flush()

	if metadataErr {
		multiError := util.MultiError{}
		if observabilityStackDefErr != nil {
			multiError.AppendError(
				fmt.Errorf("encountered an error retrieving observability stack definition info: %w", observabilityStackDefErr),
			)
		}
		if kubernetesRuntimeInstErr != nil {
			multiError.AppendError(
				fmt.Errorf("encountered an error retrieving kubernetes runtime instance info: %w", kubernetesRuntimeInstErr),
			)
		}
		if observabilityDashboardInstErr != nil {
			multiError.AppendError(
				fmt.Errorf("encountered an error retrieving observability dashboard instance info: %w", observabilityDashboardInstErr),
			)
		}
		if metricsInstErr != nil {
			multiError.AppendError(
				fmt.Errorf("encountered an error retrieving metrics instance info: %w", metricsInstErr),
			)
		}
		if loggingInstErr != nil {
			multiError.AppendError(
				fmt.Errorf("encountered an error retrieving logging instance info: %w", loggingInstErr),
			)
		}
		return multiError.Error()
	}

	return nil
}
