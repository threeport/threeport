// originally generated by 'threeport-sdk codegen api-model' but will not be regenerated - intended for modification

package cmd

import (
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
)

// outputDescribev0KubernetesRuntimeDefinitionCmd produces the plain description
// output for the 'tptctl describe kubernetes-runtime-definition' command
func outputDescribev0KubernetesRuntimeDefinitionCmd(
	kubernetesRuntimeDefinition *v0.KubernetesRuntimeDefinition,
	kubernetesRuntimeDefinitionConfig *config.KubernetesRuntimeDefinitionConfig,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	// describe AWS EKS kubernetes runtime definition
	kubernetesRuntimeStatus, err := kubernetesRuntimeDefinitionConfig.KubernetesRuntimeDefinition.Describe(
		apiClient,
		apiEndpoint,
	)
	if err != nil {
		return fmt.Errorf("failed to describe AWS EKS kubernetes runtime definition: %w", err)
	}

	// output describe details
	fmt.Printf(
		"* KubernetesRuntimeDefinition Name: %s\n",
		kubernetesRuntimeDefinitionConfig.KubernetesRuntimeDefinition.Name,
	)
	fmt.Printf(
		"* Created: %s\n",
		*kubernetesRuntimeDefinition.CreatedAt,
	)
	fmt.Printf(
		"* Last Modified: %s\n",
		*kubernetesRuntimeDefinition.UpdatedAt,
	)
	if len(*kubernetesRuntimeStatus.KubernetesRuntimeInstances) == 0 {
		fmt.Println("* No kubernetes runtime instances currently derived from this definition.")
	} else {
		fmt.Println("* Derived KubernetesRuntime Instances:")
		for _, kubernetesRuntimeInst := range *kubernetesRuntimeStatus.KubernetesRuntimeInstances {
			fmt.Printf("  * %s\n", *kubernetesRuntimeInst.Name)
		}
	}

	return nil
}

// outputDescribev0KubernetesRuntimeInstanceCmd produces the plain description
// output for the 'tptctl describe kubernetes-runtime-instance' command
func outputDescribev0KubernetesRuntimeInstanceCmd(
	kubernetesRuntimeInstance *v0.KubernetesRuntimeInstance,
	kubernetesRuntimeInstanceConfig *config.KubernetesRuntimeInstanceConfig,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	// describe AWS EKS kubernetes runtime instance
	kubernetesRuntimeStatus, err := kubernetesRuntimeInstanceConfig.KubernetesRuntimeInstance.Describe(
		apiClient,
		apiEndpoint,
	)
	if err != nil {
		return fmt.Errorf("failed to describe AWS EKS kubernetes runtime definition: %w", err)
	}

	// output describe details
	fmt.Printf(
		"* KubernetesRuntimeInstance Name: %s\n",
		kubernetesRuntimeInstanceConfig.KubernetesRuntimeInstance.Name,
	)
	fmt.Printf(
		"* Created: %s\n",
		*kubernetesRuntimeInstance.CreatedAt,
	)
	fmt.Printf(
		"* Last Modified: %s\n",
		*kubernetesRuntimeInstance.UpdatedAt,
	)
	fmt.Printf(
		"* Associated KubernetesRuntimeDefinition: %s\n",
		*kubernetesRuntimeStatus.KubernetesRuntimeDefinition.Name,
	)

	return nil
}
