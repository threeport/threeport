// originally generated by 'threeport-sdk codegen api-model' but will not be regenerated - intended for modification

package cmd

import (
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	config "github.com/threeport/threeport/pkg/config/v0"
)

// outputDescribev0TerraformDefinitionCmd produces the plain description
// output for the 'tptctl describe terraform-definition' command
func outputDescribev0TerraformDefinitionCmd(
	terraformDefinition *v0.TerraformDefinition,
	terraformDefinitionConfig *config.TerraformDefinitionConfig,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	// describe terraform definition
	terraformStatus, err := terraformDefinitionConfig.TerraformDefinition.Describe(apiClient, apiEndpoint)
	if err != nil {
		return fmt.Errorf("failed to describe terraform definition: %w", err)
	}

	// output describe details
	fmt.Printf(
		"* TerraformDefinition Name: %s\n",
		terraformDefinitionConfig.TerraformDefinition.Name,
	)
	fmt.Printf(
		"* Created: %s\n",
		*terraformDefinition.CreatedAt,
	)
	fmt.Printf(
		"* Last Modified: %s\n",
		*terraformDefinition.UpdatedAt,
	)
	if len(*terraformStatus.TerraformInstances) == 0 {
		fmt.Println("* No terraform instances currently derived from this definition.")
	} else {
		fmt.Println("* Derived Terraform Instances:")
		for _, terraformInst := range *terraformStatus.TerraformInstances {
			fmt.Printf("  * %s\n", *terraformInst.Name)
		}
	}

	return nil
}

// outputDescribev0TerraformInstanceCmd produces the plain description
// output for the 'tptctl describe terraform-instance' command
func outputDescribev0TerraformInstanceCmd(
	terraformInstance *v0.TerraformInstance,
	terraformInstanceConfig *config.TerraformInstanceConfig,
	apiClient *http.Client,
	apiEndpoint string,
) error {
	// describe terraform instance
	terraformStatus, err := terraformInstanceConfig.TerraformInstance.Describe(
		apiClient,
		apiEndpoint,
	)
	if err != nil {
		return fmt.Errorf("failed to describe terraform definition: %w", err)
	}

	// output describe details
	fmt.Printf(
		"* TerraformInstance Name: %s\n",
		terraformInstanceConfig.TerraformInstance.Name,
	)
	fmt.Printf(
		"* Created: %s\n",
		*terraformInstance.CreatedAt,
	)
	fmt.Printf(
		"* Last Modified: %s\n",
		*terraformInstance.UpdatedAt,
	)
	fmt.Printf(
		"* Associated TerraformDefinition: %s\n",
		*terraformStatus.TerraformDefinition.Name,
	)

	return nil
}
