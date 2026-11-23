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
	plane := &ControlPlaneInstance{
		IsSelf:                      util.Ptr(true),
		KubernetesRuntimeInstanceID: host.ID,
	}
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

// TestControlPlaneInstanceRefuseDeleteFromParent allows a parent to delete a
// child while the parent's own runtime is still stored.
func TestControlPlaneInstanceRefuseDeleteFromParent(t *testing.T) {
	db := setupKubernetesRuntimeValidateDB(t)

	// the parent and the child run on different runtimes
	parentHost := createValidKRI(t, db, "Local")
	childHost := &KubernetesRuntimeInstance{
		Instance:                      Instance{Name: util.Ptr("child-host")},
		Location:                      util.Ptr("Local"),
		KubernetesRuntimeDefinitionID: parentHost.KubernetesRuntimeDefinitionID,
	}
	require.NoError(t, db.Create(childHost).Error)
	child := &ControlPlaneInstance{
		IsSelf:                      util.Ptr(false),
		KubernetesRuntimeInstanceID: childHost.ID,
	}

	// a missing reader is an error, not an allowed delete
	err := child.RefuseDelete(db)
	require.Error(t, err)
	var missing *DeleteRefusedError
	require.NotErrorAs(t, err, &missing)

	// an empty reader allows the delete while the parent's runtime remains
	previous := ChildRuntimeNames
	t.Cleanup(func() { ChildRuntimeNames = previous })
	ChildRuntimeNames = func(*ControlPlaneInstance) ([]string, error) {
		return nil, nil
	}
	require.NoError(t, child.RefuseDelete(db))

	// a workload runtime stored for the parent does not block the child either
	workload := &KubernetesRuntimeInstance{
		Instance:                      Instance{Name: util.Ptr("workload")},
		Location:                      util.Ptr("Local"),
		KubernetesRuntimeDefinitionID: parentHost.KubernetesRuntimeDefinitionID,
	}
	require.NoError(t, db.Create(workload).Error)
	require.NoError(t, child.RefuseDelete(db))

	// a runtime the child itself manages does block the delete
	ChildRuntimeNames = func(*ControlPlaneInstance) ([]string, error) {
		return []string{"owned-by-child"}, nil
	}
	err = child.RefuseDelete(db)
	var childRefused *DeleteRefusedError
	require.ErrorAs(t, err, &childRefused)
	assert.Contains(t, err.Error(), "owned-by-child")
	assert.NotContains(t, err.Error(), "workload")
	assert.NotContains(t, err.Error(), "test-kri")

	// those runtimes do block deletion of the parent
	err = (&ControlPlaneInstance{
		IsSelf:                      util.Ptr(true),
		KubernetesRuntimeInstanceID: parentHost.ID,
	}).RefuseDelete(db)
	var refused *DeleteRefusedError
	require.ErrorAs(t, err, &refused)
	assert.Contains(t, err.Error(), "workload")
	assert.Contains(t, err.Error(), "child-host")
	assert.NotContains(t, err.Error(), "test-kri")
}
