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

// TestAKubernetesRuntimeDefinitionsVersionIsCheckedAndUpgraded covers
// KubernetesVersion on a kubernetes runtime definition: checked on create,
// left unset when none is given, and changed on either update shape only to
// a supported version no older than the one it replaces, on a provider
// Threeport upgrades clusters on.
func TestAKubernetesRuntimeDefinitionsVersionIsCheckedAndUpgraded(t *testing.T) {
	db := setupKubernetesApiAccessDB(t)
	create := func(name, provider string, version *string) *KubernetesRuntimeDefinition {
		definition := newKRD(name, provider, nil, nil)
		definition.KubernetesVersion = version
		require.NoError(t, db.Create(definition).Error)
		return definition
	}
	load := func(id *uint) *KubernetesRuntimeDefinition {
		var loaded KubernetesRuntimeDefinition
		require.NoError(t, db.Session(&gorm.Session{}).First(&loaded, *id).Error)
		return &loaded
	}
	patch := func(id *uint, version *string) error {
		return db.Session(&gorm.Session{}).Model(load(id)).Updates(&KubernetesRuntimeDefinition{KubernetesVersion: version}).Error
	}
	replace := func(id *uint, version *string) error {
		replaced := load(id)
		replaced.KubernetesVersion = version
		return db.Session(&gorm.Session{}).Save(replaced).Error
	}

	unset := create("unset", KubernetesRuntimeInfraProviderGKE, nil)
	assert.Nil(t, load(unset.ID).KubernetesVersion,
		"the default is recorded on the provider definition, so a cluster tptctl didn't create keeps no version it may not run")

	unsupported := newKRD("unsupported", KubernetesRuntimeInfraProviderOKE, nil, nil)
	unsupported.KubernetesVersion = util.Ptr("1.32")
	assert.ErrorContains(t, db.Create(unsupported).Error, `KubernetesVersion "1.32" is not supported`)
	kind := newKRD("kind", KubernetesRuntimeInfraProviderKind, nil, nil)
	kind.KubernetesVersion = util.Ptr(DefaultKubernetesVersion)
	assert.ErrorContains(t, db.Create(kind).Error, "cannot be set on a kind cluster")

	// EKS and GKE clusters are upgraded, on either update shape
	for _, provider := range []string{KubernetesRuntimeInfraProviderEKS, KubernetesRuntimeInfraProviderGKE} {
		patched := create("patched-"+provider, provider, util.Ptr("1.35"))
		require.NoError(t, patch(patched.ID, util.Ptr("1.36")), provider)
		assert.Equal(t, "1.36", *load(patched.ID).KubernetesVersion)
		replaced := create("replaced-"+provider, provider, util.Ptr("1.35"))
		require.NoError(t, replace(replaced.ID, util.Ptr("1.36")), provider)
		assert.Equal(t, "1.36", *load(replaced.ID).KubernetesVersion)

		// not downgraded, cleared, or moved to a version Threeport doesn't support
		assert.ErrorContains(t, patch(patched.ID, util.Ptr("1.35")), `KubernetesVersion "1.35" is older than "1.36": a cluster can't be downgraded`)
		assert.ErrorContains(t, replace(patched.ID, util.Ptr("1.35")), "can't be downgraded")
		assert.ErrorContains(t, replace(patched.ID, nil), "KubernetesVersion cannot be cleared")
		assert.ErrorContains(t, patch(patched.ID, util.Ptr("1.37")), `KubernetesVersion "1.37" is not supported`)
		assert.Equal(t, "1.36", *load(patched.ID).KubernetesVersion)
	}

	// a definition with no version runs the default, which it can't go below
	assert.ErrorContains(t, patch(unset.ID, util.Ptr("1.35")), `older than "1.36"`)
	require.NoError(t, patch(unset.ID, util.Ptr("1.36")), "the default itself is no change to its clusters")

	// OKE clusters aren't upgraded yet
	oke := create("oke", KubernetesRuntimeInfraProviderOKE, util.Ptr("1.35"))
	assert.ErrorContains(t, patch(oke.ID, util.Ptr("1.36")), ErrMsgKubernetesVersionImmutable)
	assert.ErrorContains(t, replace(oke.ID, util.Ptr("1.36")), ErrMsgKubernetesVersionImmutable)

	held := create("held", KubernetesRuntimeInfraProviderEKS, util.Ptr("1.35"))
	require.NoError(t, patch(held.ID, util.Ptr("1.35")), "sending the version it holds is not a change")
	require.NoError(t, replace(held.ID, util.Ptr("1.35")), "nor is a replace that keeps it")
	require.NoError(t, db.Session(&gorm.Session{}).Model(load(held.ID)).Updates(&KubernetesRuntimeDefinition{NodeMaximum: util.Ptr(12)}).Error)
	assert.Equal(t, "1.35", *load(held.ID).KubernetesVersion)
}

// TestTheProviderDefinitionsVersionIsCheckedAndKept covers KubernetesVersion
// on the EKS, GKE and OKE definitions, which record the version their
// clusters are created on, and on EKS and GKE the version they are upgraded
// to.
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
			require.NoError(t, db.Session(&gorm.Session{}).Model(created).Updates(build(util.Ptr("1.35"))).Error)
			err := db.Session(&gorm.Session{}).Model(created).Updates(build(util.Ptr("1.36"))).Error
			if name == "OKE" {
				assert.ErrorContains(t, err, ErrMsgKubernetesVersionImmutable, "OKE clusters aren't upgraded yet")
				return
			}
			require.NoError(t, err, "the kubernetes runtime definition's controller records the upgraded version")
			err = db.Session(&gorm.Session{}).Model(created).Updates(build(util.Ptr("1.35"))).Error
			assert.ErrorContains(t, err, "can't be downgraded")
		})
	}
}

// TestKubernetesVersionsAreComparedByNumber covers comparing minor versions
// and the version an upgrade moves a cluster to.
func TestKubernetesVersionsAreComparedByNumber(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"1.35", "1.36", -1},
		{"1.36", "1.36", 0},
		{"1.100", "1.99", 1},
		{"2.0", "1.99", 1},
	} {
		got, err := CompareKubernetesVersions(c.a, c.b)
		require.NoError(t, err)
		assert.Equal(t, c.want, got, "%s against %s", c.a, c.b)
	}
	for _, bad := range []string{"1", "v1.36", "1.36.4", "1.x", ""} {
		_, err := CompareKubernetesVersions(bad, "1.36")
		assert.Error(t, err, bad)
		_, err = CompareKubernetesVersions("1.36", bad)
		assert.Error(t, err, bad)
	}

	next, err := NextKubernetesVersion("1.35")
	require.NoError(t, err)
	assert.Equal(t, "1.36", next)
	next, err = NextKubernetesVersion("1.99")
	require.NoError(t, err)
	assert.Equal(t, "1.100", next)
	_, err = NextKubernetesVersion("1.36.4")
	assert.Error(t, err)
}
