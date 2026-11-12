package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestAKubernetesVersionIsASupportedMinorVersion covers the versions a
// definition may ask for on each provider.
func TestAKubernetesVersionIsASupportedMinorVersion(t *testing.T) {
	require.Contains(t, SupportedKubernetesVersions, DefaultKubernetesVersion)

	for _, provider := range []string{
		KubernetesRuntimeInfraProviderEKS,
		KubernetesRuntimeInfraProviderGKE,
		KubernetesRuntimeInfraProviderOKE,
	} {
		assert.NoError(t, ValidateKubernetesVersion(provider, nil), provider)
		for _, version := range SupportedKubernetesVersions {
			assert.NoError(t, ValidateKubernetesVersion(provider, util.Ptr(version)), provider)
		}
		for _, version := range []string{"1.32", "1.36.4", "v1.36", "1.99", ""} {
			err := ValidateKubernetesVersion(provider, util.Ptr(version))
			if assert.Error(t, err, "%s %q", provider, version) {
				assert.Contains(t, err.Error(), "is not supported: use one of 1.35, 1.36")
			}
		}
	}

	assert.NoError(t, ValidateKubernetesVersion(KubernetesRuntimeInfraProviderKind, nil))
	assert.ErrorContains(t,
		ValidateKubernetesVersion(KubernetesRuntimeInfraProviderKind, util.Ptr(DefaultKubernetesVersion)),
		"cannot be set on a kind cluster",
	)

	assert.Equal(t, DefaultKubernetesVersion, KubernetesVersionOrDefault(nil))
	assert.Equal(t, DefaultKubernetesVersion, KubernetesVersionOrDefault(util.Ptr("")))
	assert.Equal(t, "1.35", KubernetesVersionOrDefault(util.Ptr("1.35")))
}

// TestAKubernetesRuntimeDefinitionsVersionIsCheckedAndKept covers
// KubernetesVersion on a kubernetes runtime definition: checked on create,
// left unset when none is given, and refused a change on either update shape.
func TestAKubernetesRuntimeDefinitionsVersionIsCheckedAndKept(t *testing.T) {
	db := setupKubernetesApiAccessDB(t)

	asked := newKRD("asked", KubernetesRuntimeInfraProviderEKS, nil, nil)
	asked.KubernetesVersion = util.Ptr("1.35")
	require.NoError(t, db.Create(asked).Error)

	unset := newKRD("unset", KubernetesRuntimeInfraProviderGKE, nil, nil)
	require.NoError(t, db.Create(unset).Error)
	var stored KubernetesRuntimeDefinition
	require.NoError(t, db.First(&stored, *unset.ID).Error)
	assert.Nil(t, stored.KubernetesVersion,
		"the default is recorded on the provider definition, so a cluster tptctl didn't create keeps no version it may not run")

	unsupported := newKRD("unsupported", KubernetesRuntimeInfraProviderOKE, nil, nil)
	unsupported.KubernetesVersion = util.Ptr("1.32")
	assert.ErrorContains(t, db.Create(unsupported).Error, `KubernetesVersion "1.32" is not supported`)
	kind := newKRD("kind", KubernetesRuntimeInfraProviderKind, nil, nil)
	kind.KubernetesVersion = util.Ptr(DefaultKubernetesVersion)
	assert.ErrorContains(t, db.Create(kind).Error, "cannot be set on a kind cluster")

	load := func(id *uint) *KubernetesRuntimeDefinition {
		var loaded KubernetesRuntimeDefinition
		require.NoError(t, db.First(&loaded, *id).Error)
		return &loaded
	}
	err := db.Model(load(asked.ID)).Updates(&KubernetesRuntimeDefinition{KubernetesVersion: util.Ptr("1.36")}).Error
	assert.ErrorContains(t, err, ErrMsgKubernetesVersionImmutable)
	replaced := load(asked.ID)
	replaced.KubernetesVersion = util.Ptr("1.36")
	assert.ErrorContains(t, db.Save(replaced).Error, ErrMsgKubernetesVersionImmutable)
	cleared := load(asked.ID)
	cleared.KubernetesVersion = nil
	assert.ErrorContains(t, db.Save(cleared).Error, ErrMsgKubernetesVersionImmutable)
	err = db.Model(load(unset.ID)).Updates(&KubernetesRuntimeDefinition{KubernetesVersion: util.Ptr("1.36")}).Error
	assert.ErrorContains(t, err, ErrMsgKubernetesVersionImmutable, "nor can one be set later, since the clusters run the default")

	require.NoError(t, db.Model(load(asked.ID)).Updates(&KubernetesRuntimeDefinition{KubernetesVersion: util.Ptr("1.35")}).Error,
		"sending the version it holds is not a change")
	require.NoError(t, db.Save(load(asked.ID)).Error, "nor is a replace that keeps it")
	require.NoError(t, db.Model(load(asked.ID)).Updates(&KubernetesRuntimeDefinition{NodeMaximum: util.Ptr(12)}).Error)
	assert.Equal(t, "1.35", *load(asked.ID).KubernetesVersion)
}

// TestTheProviderDefinitionsVersionIsCheckedAndKept covers KubernetesVersion
// on the EKS, GKE and OKE definitions, which record the version their
// clusters are created on.
func TestTheProviderDefinitionsVersionIsCheckedAndKept(t *testing.T) {
	krdID := util.Ptr(uint(1))
	common := func(name string) Definition { return Definition{Name: util.Ptr(name)} }
	cases := map[string]func(version *string) any{
		"EKS": func(version *string) any {
			return &AwsEksKubernetesRuntimeDefinition{
				Definition: common("eks"), ZoneCount: util.Ptr(2), DefaultNodeGroupInstanceType: util.Ptr("m5.large"),
				DefaultNodeGroupInitialSize: util.Ptr(2), DefaultNodeGroupMinimumSize: util.Ptr(0), DefaultNodeGroupMaximumSize: util.Ptr(5),
				KubernetesRuntimeDefinitionID: krdID, KubernetesVersion: version,
			}
		},
		"GKE": func(version *string) any {
			return &GcpGkeKubernetesRuntimeDefinition{
				Definition: common("gke"), ZoneCount: util.Ptr(2), DefaultNodeGroupInstanceType: util.Ptr("e2-standard-4"),
				DefaultNodeGroupInitialSize: util.Ptr(2), DefaultNodeGroupMinimumSize: util.Ptr(0), DefaultNodeGroupMaximumSize: util.Ptr(5),
				KubernetesRuntimeDefinitionID: krdID, KubernetesVersion: version,
			}
		},
		"OKE": func(version *string) any {
			return &OciOkeKubernetesRuntimeDefinition{
				Definition: common("oke"), WorkerNodeShape: util.Ptr("VM.Standard.E4.Flex"), WorkerNodeInitialCount: util.Ptr(int32(2)),
				KubernetesRuntimeDefinitionID: krdID, KubernetesVersion: version,
			}
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			db := setupKubernetesApiAccessDB(t)
			assert.ErrorContains(t, db.Create(build(util.Ptr("1.30"))).Error, `KubernetesVersion "1.30" is not supported`)

			created := build(util.Ptr("1.35"))
			require.NoError(t, db.Create(created).Error)
			err := db.Model(created).Updates(build(util.Ptr("1.36"))).Error
			assert.ErrorContains(t, err, ErrMsgKubernetesVersionImmutable)
			require.NoError(t, db.Session(&gorm.Session{}).Model(created).Updates(build(util.Ptr("1.35"))).Error)
		})
	}
}
