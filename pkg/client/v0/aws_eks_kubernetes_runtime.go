package v0

import (
	"fmt"
	"net/http"

	"github.com/nukleros/aws-builder/pkg/eks"
)

// GetResourceInventoryByK8sRuntimeInst returns the DNS management IAM role arn.
func GetResourceInventoryByK8sRuntimeInst(
	apiClient *http.Client,
	apiAddr string,
	kubernetesRuntimeInstanceId *uint,
	// ) (*resource.ResourceInventory, error) {
) (*eks.EksInventory, error) {

	// get dns management role arn
	aekri, err := GetAwsEksKubernetesRuntimeInstanceByK8sRuntimeInst(apiClient, apiAddr, *kubernetesRuntimeInstanceId)
	if err != nil {
		return nil, fmt.Errorf("failed to get aws eks kubernetes runtime instance: %w", err)
	}

	if aekri.ResourceInventory == nil {
		return nil, fmt.Errorf("aws eks kubernetes runtime instance does not have a resource inventory")
	}

	// unmarshal the inventory into an ResourceInventory object
	//var inventory resource.ResourceInventory
	//err = resource.UnmarshalInventory(
	//	[]byte(*aekri.ResourceInventory),
	//	&inventory,
	//)
	//if err != nil {
	var inventory eks.EksInventory
	if err := inventory.Unmarshal([]byte(*aekri.ResourceInventory)); err != nil {
		return nil, fmt.Errorf("failed to unmarshal resource inventory: %w", err)
	}

	return &inventory, nil
}
