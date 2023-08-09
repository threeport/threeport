package v0

import (
	"fmt"
	"net/http"

	"github.com/nukleros/eks-cluster/pkg/resource"
)

// GetResourceInventoryByK8sRuntimeInst returns the DNS management IAM role arn.
func GetResourceInventoryByK8sRuntimeInst(
	apiClient *http.Client,
	apiAddr string,
	kubernetesRuntimeInstanceId *uint,
) (*resource.ResourceInventory, error) {

	// get dns management role arn
	aekri, err := GetAwsEksKubernetesRuntimeInstanceByK8sRuntimeInst(apiClient, apiAddr, *kubernetesRuntimeInstanceId)
	if err != nil {
		return nil, fmt.Errorf("failed to get aws eks kubernetes runtime instance: %w", err)
	}

	// unmarshal the inventory into an ResourceInventory object
	var inventory resource.ResourceInventory
	err = resource.UnmarshalInventory(
		[]byte(*aekri.ResourceInventory),
		&inventory,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal resource inventory: %w", err)
	}

	return &inventory, nil
}
