package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestAnEksDefinitionsNodeGroupPerZoneIsKept covers NodeGroupPerZone on an
// EKS definition: unset is false, and a change is refused on either update
// shape, since it would replace the clusters' node groups.
func TestAnEksDefinitionsNodeGroupPerZoneIsKept(t *testing.T) {
	db := setupKubernetesApiAccessDB(t)
	build := func(name string, runtimeID uint, perZone *bool) *AwsEksKubernetesRuntimeDefinition {
		return &AwsEksKubernetesRuntimeDefinition{
			Definition: Definition{Name: util.Ptr(name)}, ZoneCount: util.Ptr(3), DefaultNodeGroupInstanceType: util.Ptr("m8i.large"),
			DefaultNodeGroupInitialSize: util.Ptr(1), DefaultNodeGroupMinimumSize: util.Ptr(1), DefaultNodeGroupMaximumSize: util.Ptr(10),
			KubernetesRuntimeDefinitionID: util.Ptr(runtimeID), NodeGroupPerZone: perZone,
		}
	}
	load := func(id *uint) *AwsEksKubernetesRuntimeDefinition {
		var loaded AwsEksKubernetesRuntimeDefinition
		require.NoError(t, db.Session(&gorm.Session{}).First(&loaded, *id).Error)
		return &loaded
	}

	unset := build("unset", 1, nil)
	require.NoError(t, db.Create(unset).Error)
	assert.False(t, *load(unset.ID).NodeGroupPerZone)

	perZone := build("per-zone", 2, util.Ptr(true))
	require.NoError(t, db.Create(perZone).Error)
	err := db.Session(&gorm.Session{}).Model(load(perZone.ID)).Updates(map[string]any{"NodeGroupPerZone": false}).Error
	assert.ErrorContains(t, err, "NodeGroupPerZone cannot be changed after creation")
	replaced := load(perZone.ID)
	replaced.NodeGroupPerZone = util.Ptr(false)
	assert.ErrorContains(t, db.Session(&gorm.Session{}).Save(replaced).Error, "NodeGroupPerZone cannot be changed after creation")

	require.NoError(t, db.Session(&gorm.Session{}).Save(load(perZone.ID)).Error, "a replace that keeps it")
	require.NoError(t, db.Session(&gorm.Session{}).Model(load(perZone.ID)).Updates(
		&AwsEksKubernetesRuntimeDefinition{DefaultNodeGroupMaximumSize: util.Ptr(12)},
	).Error)
	assert.True(t, *load(perZone.ID).NodeGroupPerZone)

	// a client that doesn't know the field sends none, which is one node
	// group spanning every zone, as false is
	omitted := load(unset.ID)
	omitted.NodeGroupPerZone = nil
	omitted.DefaultNodeGroupMaximumSize = util.Ptr(12)
	require.NoError(t, db.Session(&gorm.Session{}).Save(omitted).Error, "a replace without the field keeps one node group")
	perZoneOmitted := load(perZone.ID)
	perZoneOmitted.NodeGroupPerZone = nil
	assert.ErrorContains(t, db.Session(&gorm.Session{}).Save(perZoneOmitted).Error, "NodeGroupPerZone cannot be changed after creation",
		"which would move a cluster with a node group per zone to one")
	err = db.Session(&gorm.Session{}).Model(load(unset.ID)).Updates(map[string]any{"NodeGroupPerZone": true}).Error
	assert.ErrorContains(t, err, "NodeGroupPerZone cannot be changed after creation")
}
