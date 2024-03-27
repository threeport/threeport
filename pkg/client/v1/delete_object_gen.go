// generated by 'threeport-sdk gen' for API object deletion boilerplate - do not edit

package v1

import (
	"fmt"
	"net/http"
)

// DeleteObjectByTypeAndID deletes an instance given a string representation of its type and ID.
func DeleteObjectByTypeAndID(apiClient *http.Client, apiAddr string, objectType string, id uint) error {

	switch objectType {
	case "v1.AttachedObjectReference":
		if _, err := DeleteAttachedObjectReference(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete AttachedObjectReference: %w", err)
		}
	case "v1.WorkloadInstance":
		if _, err := DeleteWorkloadInstance(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete WorkloadInstance: %w", err)
		}

	}

	return nil
}