// originally generated by 'threeport-sdk codegen api-model' but will not be regenerated - intended for modification

package cmd

import (
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"

	"github.com/threeport/threeport/internal/workload/status"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// outputDescribev0HelmWorkloadDefinitionCmd produces the plain description
// output for the 'tptctl describe helm-workload-definition' command
func outputDescribev0HelmWorkloadDefinitionCmd(
	helmWorkloadDefinition *v0.HelmWorkloadDefinition,
	helmWorkloadDefinitionConfig *config.HelmWorkloadDefinitionConfig,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	// describe helm workload definition
	helmWorkloadStatus, err := helmWorkloadDefinitionConfig.HelmWorkloadDefinition.Describe(apiClient, apiEndpoint)
	if err != nil {
		return fmt.Errorf("failed to describe helm workload definition: %w", err)
	}

	// output describe details
	fmt.Printf(
		"* HelmWorkloadDefinition Name: %s\n",
		helmWorkloadDefinitionConfig.HelmWorkloadDefinition.Name,
	)
	fmt.Printf(
		"* Created: %s\n",
		*helmWorkloadDefinition.CreatedAt,
	)
	fmt.Printf(
		"* Last Modified: %s\n",
		*helmWorkloadDefinition.UpdatedAt,
	)
	if len(*helmWorkloadStatus.HelmWorkloadInstances) == 0 {
		fmt.Println("* No helm workload instances currently derived from this definition.")
	} else {
		fmt.Println("* Derived HelmWorkload Instances:")
		for _, helmWorkloadInst := range *helmWorkloadStatus.HelmWorkloadInstances {
			fmt.Printf("  * %s\n", *helmWorkloadInst.Name)
		}
	}

	return nil
}

// outputDescribev0HelmWorkloadInstanceCmd produces the plain description
// output for the 'tptctl describe helm-workload-instance' command
func outputDescribev0HelmWorkloadInstanceCmd(
	helmWorkloadInstance *v0.HelmWorkloadInstance,
	helmWorkloadInstanceConfig *config.HelmWorkloadInstanceConfig,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	// describe helm workload instance
	helmWorkloadStatus, err := helmWorkloadInstanceConfig.HelmWorkloadInstance.Describe(
		apiClient,
		apiEndpoint,
	)
	if err != nil {
		return fmt.Errorf("failed to describe helm workload instance: %w", err)
	}

	// output describe details
	fmt.Printf(
		"* HelmWorkloadInstance Name: %s\n",
		helmWorkloadInstanceConfig.HelmWorkloadInstance.Name,
	)
	fmt.Printf(
		"* Created: %s\n",
		*helmWorkloadInstance.CreatedAt,
	)
	fmt.Printf(
		"* Last Modified: %s\n",
		*helmWorkloadInstance.UpdatedAt,
	)
	fmt.Printf(
		"* HelmWorkloadInstance Status: %s\n",
		helmWorkloadStatus.Status,
	)
	if helmWorkloadStatus.Reason != "" {
		fmt.Printf(
			"HelmWorkloadInstance Status Reason: %s\n",
			helmWorkloadStatus.Reason,
		)
	}
	if len(helmWorkloadStatus.Events) > 0 && helmWorkloadStatus.Status != status.WorkloadInstanceStatusHealthy {
		cli.Warning("Failed & Warning Events:")
		writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
		fmt.Fprintln(writer, "TYPE\t REASON\t MESSAGE\t AGE")
		for _, event := range helmWorkloadStatus.Events {
			fmt.Fprintln(
				writer, *event.Type, "\t", *event.Reason, "\t", *event.Message, "\t",
				util.GetAge(event.Timestamp),
			)
		}
		writer.Flush()
	}

	return nil
}
