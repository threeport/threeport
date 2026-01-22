package v0

import (
	"fmt"

	"gorm.io/gorm"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// BeforeDelete validates a delete request on a gcp account
// deletion to ensure deletion is possible.  A GcpAccount may not
// be deleted if it has related GcpGkeKubernetesRuntimeInstances.
func (g *GcpAccount) BeforeDelete(tx *gorm.DB) error {
	var gcpGkeKubernetesRuntimeInstances []GcpGkeKubernetesRuntimeInstance
	if result := tx.Where(
		&GcpGkeKubernetesRuntimeInstance{GcpAccountID: g.ID},
	).Find(&gcpGkeKubernetesRuntimeInstances); result.Error != nil {
		return fmt.Errorf(
			"failed to query gcp gke kubernetes runtime instances for gcp account %s",
			*g.Name,
		)
	}

	if len(gcpGkeKubernetesRuntimeInstances) > 0 {
		return util.NewBadRequestError(
			fmt.Sprintf(
				"gcp account %s has related gcp gke kubernetes runtime instances - cannot be deleted",
				*g.Name,
			),
		)
	}
	return nil
}
