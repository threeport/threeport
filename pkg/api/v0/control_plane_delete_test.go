package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestControlPlaneInstanceRefuseDelete allows a delete that leaves only the
// runtime the control plane runs on, and refuses one that leaves another.
func TestControlPlaneInstanceRefuseDelete(t *testing.T) {
	db := setupKubernetesRuntimeValidateDB(t)

	// the host runtime does not block the delete
	host := createValidKRI(t, db, "Local")
	plane := &ControlPlaneInstance{KubernetesRuntimeInstanceID: host.ID}
	err := plane.RefuseDelete(db)
	require.NoError(t, err)

	// another runtime blocks it
	owned := &KubernetesRuntimeInstance{
		Instance:                      Instance{Name: util.Ptr("workload")},
		Location:                      util.Ptr("Local"),
		KubernetesRuntimeDefinitionID: host.KubernetesRuntimeDefinitionID,
	}
	require.NoError(t, db.Create(owned).Error)
	err = plane.RefuseDelete(db)
	var refused *DeleteRefusedError
	require.ErrorAs(t, err, &refused)
	assert.Contains(t, err.Error(), ErrMsgDeleteBlocked)
	assert.Contains(t, err.Error(), "workload")

	// a deleted runtime does not block the delete
	require.NoError(t, db.Delete(owned).Error)
	require.NoError(t, plane.RefuseDelete(db))
}
