package controlplane

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	"github.com/threeport/threeport/pkg/encryption/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestAChildKeepsItsRuntimesRangeButNotItsParentsPool covers what a child
// control plane is given of the network ranges its parent allocated.  A
// runtime keeps its range on the child, so the child never hands that range
// out.  The parent's own row on the child carries no pool: the parent
// allocates from it in its own database, and on the child it would read as a
// pool the child's ranges must stay clear of, which the child's own runtime,
// given a range from that pool, is not.
func TestAChildKeepsItsRuntimesRangeButNotItsParentsPool(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)

	parent := newFakeAPI(t, 1)
	parent.seed(v0.PathControlPlaneDefinitions, map[string]any{"ID": 2, "Name": "parent-definition"})
	parent.seed(v0.PathKubernetesRuntimeDefinitions, map[string]any{"ID": 3, "Name": "parent-runtime-definition", "InfraProvider": v0.KubernetesRuntimeInfraProviderKind})
	parent.seed(v0.PathKubernetesRuntimeInstances, map[string]any{
		"ID": 4, "Name": "parent-runtime", "KubernetesRuntimeDefinitionID": 3, "NetworkCidr": "10.16.0.0/16",
	})
	child := newFakeAPI(t, 500)
	o := &runtimeOnboarder{parent: parent.target(), child: child.target(), encryptionKey: key}

	pool := datatypes.JSON(`["10.16.0.0/12"]`)
	parentInstance := &v0.ControlPlaneInstance{
		Common:                      v0.Common{ID: util.Ptr(uint(1))},
		Instance:                    v0.Instance{Name: util.Ptr("parent")},
		Namespace:                   util.Ptr("threeport-control-plane"),
		IsSelf:                      util.Ptr(true),
		NetworkCidrPool:             &pool,
		ControlPlaneDefinitionID:    util.Ptr(uint(2)),
		KubernetesRuntimeInstanceID: util.Ptr(uint(4)),
	}
	_, err = copyManagingParent(o, parentInstance)
	require.NoError(t, err)

	runtimes := child.createdOn(v0.PathKubernetesRuntimeInstances)
	require.Len(t, runtimes, 1)
	assert.Equal(t, "10.16.0.0/16", runtimes[0]["NetworkCidr"])

	instances := child.createdOn(v0.PathControlPlaneInstances)
	require.Len(t, instances, 1)
	assert.Nil(t, instances[0]["NetworkCidrPool"])
	assert.Equal(t, false, instances[0]["IsSelf"])
	assert.Equal(t, &pool, parentInstance.NetworkCidrPool, "the parent's own record is left as it was")
}
