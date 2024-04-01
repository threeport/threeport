// originally generated by 'threeport-sdk codegen api-model' but will not be regenerated - intended for modification

package cmd

import (
	"fmt"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
	"net/http"
	"os"
	"text/tabwriter"
)

// outputGetSecretsCmd produces the tabular output for the
// 'tptctl get secrets' command.
func outputGetSecretsCmd(
	secretInstances *[]v0.SecretInstance,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
	fmt.Fprintln(writer, "NAME\t AGE")
	for _, secretInstance := range *secretInstances {
		fmt.Fprintln(
			writer,
			*secretInstance.Name, "\t",
			util.GetAge(secretInstance.CreatedAt),
		)
	}
	writer.Flush()

	return nil
}

// outputGetSecretDefinitionsCmd produces the tabular output for the
// 'tptctl get secret-definitions' command.
func outputGetSecretDefinitionsCmd(
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

// outputGetSecretInstancesCmd produces the tabular output for the
// 'tptctl get secret-instances' command.
func outputGetSecretInstancesCmd(
	secretInstances *[]v0.SecretInstance,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	writer := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', 0)
	fmt.Fprintln(writer, "NAME\t AGE")
	for _, secretInstance := range *secretInstances {
		fmt.Fprintln(
			writer,
			*secretInstance.Name, "\t",
			util.GetAge(secretInstance.CreatedAt),
		)
	}
	writer.Flush()

	return nil
}