package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestADefinitionsNodeCountsAreChecked covers the node group sizes of an EKS
// or GKE definition, on create, patch and replace: the initial size is
// between the minimum and the maximum, and on GKE the maximum allows a node
// in each zone the nodes run in.  An update that sets none of them isn't
// checked against them.
func TestADefinitionsNodeCountsAreChecked(t *testing.T) {
	type sizes struct{ minimum, initial, maximum, zones int }
	cases := map[string]func(s sizes) any{
		"EKS": func(s sizes) any {
			return &AwsEksKubernetesRuntimeDefinition{
				Definition: Definition{Name: util.Ptr("eks")}, ZoneCount: util.Ptr(s.zones), DefaultNodeGroupInstanceType: util.Ptr("m5.large"),
				DefaultNodeGroupMinimumSize: util.Ptr(s.minimum), DefaultNodeGroupInitialSize: util.Ptr(s.initial),
				DefaultNodeGroupMaximumSize: util.Ptr(s.maximum), KubernetesRuntimeDefinitionID: util.Ptr(uint(1)),
			}
		},
		"GKE": func(s sizes) any {
			return &GcpGkeKubernetesRuntimeDefinition{
				Definition: Definition{Name: util.Ptr("gke")}, ZoneCount: util.Ptr(s.zones), DefaultNodeGroupInstanceType: util.Ptr("e2-standard-4"),
				DefaultNodeGroupMinimumSize: util.Ptr(s.minimum), DefaultNodeGroupInitialSize: util.Ptr(s.initial),
				DefaultNodeGroupMaximumSize: util.Ptr(s.maximum), KubernetesRuntimeDefinitionID: util.Ptr(uint(1)),
			}
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			db := setupKubernetesApiAccessDB(t)
			session := func() *gorm.DB { return db.Session(&gorm.Session{}) }

			assert.ErrorContains(t, session().Create(build(sizes{3, 2, 10, 3})).Error, "DefaultNodeGroupMinimumSize 3 is more than DefaultNodeGroupInitialSize 2")
			assert.ErrorContains(t, session().Create(build(sizes{0, 12, 10, 3})).Error, "DefaultNodeGroupInitialSize 12 is more than DefaultNodeGroupMaximumSize 10")
			if name == "GKE" {
				assert.ErrorContains(t, session().Create(build(sizes{0, 2, 2, 3})).Error, "DefaultNodeGroupMaximumSize 2 is less than the 3 zones the nodes run in")
			} else {
				counts, ok := eksNodeCounts(build(sizes{0, 2, 2, 3}))
				require.True(t, ok)
				assert.NoError(t, validateNodeCounts(counts), "an EKS node group spans its zones")
			}

			created := build(sizes{0, 2, 10, 3})
			require.NoError(t, session().Create(created).Error)

			// a patch is checked with the sizes it leaves as they are
			patch := func(update any) error { return session().Model(created).Updates(update).Error }
			switch name {
			case "EKS":
				assert.ErrorContains(t, patch(&AwsEksKubernetesRuntimeDefinition{DefaultNodeGroupMaximumSize: util.Ptr(1)}), "is more than DefaultNodeGroupMaximumSize 1")
				assert.ErrorContains(t, patch(&AwsEksKubernetesRuntimeDefinition{DefaultNodeGroupMinimumSize: util.Ptr(5)}), "DefaultNodeGroupMinimumSize 5 is more than DefaultNodeGroupInitialSize 2")
				assert.NoError(t, patch(&AwsEksKubernetesRuntimeDefinition{DefaultNodeGroupMaximumSize: util.Ptr(20)}))
				assert.NoError(t, patch(&AwsEksKubernetesRuntimeDefinition{DefaultNodeGroupInstanceType: util.Ptr("m5.xlarge")}))
			case "GKE":
				assert.ErrorContains(t, patch(&GcpGkeKubernetesRuntimeDefinition{DefaultNodeGroupMaximumSize: util.Ptr(1)}), "is more than DefaultNodeGroupMaximumSize 1")
				assert.ErrorContains(t, patch(&GcpGkeKubernetesRuntimeDefinition{ZoneCount: util.Ptr(30)}), "is less than the 30 zones")
				assert.NoError(t, patch(&GcpGkeKubernetesRuntimeDefinition{DefaultNodeGroupMaximumSize: util.Ptr(20)}))
				assert.NoError(t, patch(&GcpGkeKubernetesRuntimeDefinition{DefaultNodeGroupInstanceType: util.Ptr("e2-standard-8")}))
			}

			// a replace is checked as it is
			replaced := build(sizes{0, 4, 3, 3})
			switch object := replaced.(type) {
			case *AwsEksKubernetesRuntimeDefinition:
				object.ID = created.(*AwsEksKubernetesRuntimeDefinition).ID
			case *GcpGkeKubernetesRuntimeDefinition:
				object.ID = created.(*GcpGkeKubernetesRuntimeDefinition).ID
			}
			assert.ErrorContains(t, session().Save(replaced).Error, "DefaultNodeGroupInitialSize 4 is more than DefaultNodeGroupMaximumSize 3")
		})
	}
}

// TestAKubernetesRuntimeDefinitionsNodeMaximumIsChecked covers NodeMaximum
// on a kubernetes runtime definition: its clusters start with two nodes, so
// it can't be less, and a highly available GKE cluster's nodes run in three
// zones, so there it can't be less than three.
func TestAKubernetesRuntimeDefinitionsNodeMaximumIsChecked(t *testing.T) {
	db := setupKubernetesApiAccessDB(t)
	create := func(name, provider string, highAvailability bool, maximum *int) error {
		definition := newKRD(name, provider, nil, nil)
		definition.HighAvailability = util.Ptr(highAvailability)
		definition.NodeMaximum = maximum
		return db.Session(&gorm.Session{}).Create(definition).Error
	}

	assert.ErrorContains(t, create("one", KubernetesRuntimeInfraProviderEKS, false, util.Ptr(1)), "NodeMaximum 1 is less than the 2 nodes a cluster starts with")
	assert.ErrorContains(t, create("ha-gke", KubernetesRuntimeInfraProviderGKE, true, util.Ptr(2)), "NodeMaximum 2 is less than the 3 zones")
	assert.NoError(t, create("ha-eks", KubernetesRuntimeInfraProviderEKS, true, util.Ptr(2)))
	assert.NoError(t, create("zonal-gke", KubernetesRuntimeInfraProviderGKE, false, util.Ptr(2)))
	assert.NoError(t, create("ha-gke-3", KubernetesRuntimeInfraProviderGKE, true, util.Ptr(3)))

	defaulted := newKRD("defaulted", KubernetesRuntimeInfraProviderGKE, nil, nil)
	require.NoError(t, db.Session(&gorm.Session{}).Create(defaulted).Error)
	var stored KubernetesRuntimeDefinition
	require.NoError(t, db.First(&stored, *defaulted.ID).Error)
	assert.Equal(t, util.Ptr(60), stored.NodeMaximum, "a definition without one gets 60")
}
