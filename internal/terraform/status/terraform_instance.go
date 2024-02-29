package status

import (
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
)

// TerraformInstanceStatusDetail contains all the data for
// terraform instance status info.
type TerraformInstanceStatusDetail struct {
	TerraformDefinition *v0.TerraformDefinition
}

// GetTerraformInstanceStatus inspects a terraform instance
// and returns the status detials for it.
func GetTerraformInstanceStatus(
	apiClient *http.Client,
	apiEndpoint string,
	terraformInstance *v0.TerraformInstance,
) (*TerraformInstanceStatusDetail, error) {
	var terraformInstStatus TerraformInstanceStatusDetail

	// retrieve terraform definition for the instance
	terraformDef, err := client.GetTerraformDefinitionByID(
		apiClient,
		apiEndpoint,
		*terraformInstance.TerraformDefinitionID,
	)
	if err != nil {
		return &terraformInstStatus, fmt.Errorf("failed to retrieve terraform definition related to terraform instance: %w", err)
	}
	terraformInstStatus.TerraformDefinition = terraformDef

	return &terraformInstStatus, nil
}
