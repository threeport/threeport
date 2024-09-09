// originally generated by 'threeport-sdk codegen api-model' but will not be regenerated - intended for modification

package cmd

import (
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"

	client "github.com/threeport/threeport/pkg/client/v0"
)

// outputGetSecretsCmd produces the tabular output for the
// 'tptctl get secrets' command.
func outputGetSecretsCmd(
	secretInstances *[]v0.SecretInstance,
	apiClient *http.Client,
	apiEndpoint string,
) error {

	writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
	fmt.Fprintln(writer, "NAME\t AGE\t WORKLOAD INSTANCE")
	if len(*secretInstances) > 0 {
		for _, secretInstance := range *secretInstances {
			workloadInstanceName, _ := getWorkloadInstanceName(apiClient, apiEndpoint, secretInstance)

			fmt.Fprintln(
				writer,
				*secretInstance.Name, "\t",
				util.GetAge(secretInstance.CreatedAt), "\t",
				workloadInstanceName,
			)
		}
	}
	writer.Flush()

	return nil
}

// outputGetv0SecretDefinitionsCmd produces the tabular output for the
// 'tptctl get secret-definitions' command.
func outputGetv0SecretDefinitionsCmd(
	secretDefinitions *[]v0.SecretDefinition,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
	fmt.Fprintln(writer, "NAME\t AGE")
	for _, secretDefinition := range *secretDefinitions {
		fmt.Fprintln(
			writer,
			*secretDefinition.Name, "\t",
			util.GetAge(secretDefinition.CreatedAt),
		)
	}
	writer.Flush()

	return nil
}

// outputGetv0SecretInstancesCmd produces the tabular output for the
// 'tptctl get secret-instances' command.
func outputGetv0SecretInstancesCmd(
	secretInstances *[]v0.SecretInstance,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
	fmt.Fprintln(writer, "NAME\t AGE\t WORKLOAD INSTANCE")
	if len(*secretInstances) > 0 {
		for _, secretInstance := range *secretInstances {
			workloadInstanceName, _ := getWorkloadInstanceName(apiClient, apiEndpoint, secretInstance)
			fmt.Fprintln(
				writer,
				*secretInstance.Name, "\t",
				util.GetAge(secretInstance.CreatedAt), "\t",
				workloadInstanceName,
			)
		}
	}
	writer.Flush()

	return nil
}

// getWorkloadInstanceName returns the name of the workload instance
// that the secret instance is attached to.
func getWorkloadInstanceName(apiClient *http.Client, apiEndpoint string, secretInstance v0.SecretInstance) (string, error) {
	attachedObjectReferences, err := client.GetAttachedObjectReferencesByAttachedObjectID(
		apiClient,
		apiEndpoint,
		*secretInstance.ID,
	)
	if err != nil {
		return "", fmt.Errorf("failed to get attached object references: %v", err)
	}

	if len(*attachedObjectReferences) == 0 {
		return "", fmt.Errorf("expected 1 attached object reference, got %d", len(*attachedObjectReferences))
	}

	if len(*attachedObjectReferences) > 1 {
		return "", fmt.Errorf("expected 1 attached object reference, got %d", len(*attachedObjectReferences))
	}

	workloadInstance, err := client.GetWorkloadInstanceByID(
		apiClient,
		apiEndpoint,
		*(*attachedObjectReferences)[0].ObjectID,
	)
	if err != nil {
		return "", fmt.Errorf("failed to get workload instance: %v", err)
	}

	return *workloadInstance.Name, nil
}
