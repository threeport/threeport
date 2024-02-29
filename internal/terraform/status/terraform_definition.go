package status

import (
	"fmt"
	"net/http"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
)

// TerraformDefinitionStatusDetail contains all the data for terraform instance
// status info.
type TerraformDefinitionStatusDetail struct {
	TerraformInstances *[]v0.TerraformInstance
}

// GetTerraformDefinitionStatus inspects a terraform definition and returns the status
// detials for it.
func GetTerraformDefinitionStatus(
	apiClient *http.Client,
	apiEndpoint string,
	terraformDefinitionId uint,
) (*TerraformDefinitionStatusDetail, error) {
	var terraformDefStatus TerraformDefinitionStatusDetail

	// retrieve terraform instances related to terraform definition
	terraformInsts, err := client.GetTerraformInstancesByQueryString(
		apiClient,
		apiEndpoint,
		fmt.Sprintf("terraformdefinitionid=%d", terraformDefinitionId),
	)
	if err != nil {
		return &terraformDefStatus, fmt.Errorf("failed to retrieve terraform instances related to terraform definition: %w", err)
	}
	terraformDefStatus.TerraformInstances = terraformInsts

	return &terraformDefStatus, nil
}
