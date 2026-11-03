package v0

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// Installers are built once per child control plane inside one long-lived
// controller process. Whatever one installer is told to do must stay with that
// installer, or the next child installs what the previous one asked for.

func TestNewInstaller_OverridesDoNotLeakToLaterInstallers(t *testing.T) {
	originalAgentTag := ThreeportAgent.ImageTag
	originalControllerNamespace := ThreeportControllerList[0].ImageNamespace
	originalNamespace := NewInstaller().Opts.Namespace

	// the writes the control plane reconciler makes for a child's overrides
	a := NewInstaller(Namespace("child-a"))
	a.Opts.AgentInfo.ImageTag = "tag-from-child-a"
	a.Opts.ControllerList[0].ImageNamespace = "registry-from-child-a"
	disabled := false
	a.Opts.ControllerList[1].Enabled = &disabled
	a.Opts.RestApiInfo.ImageTag = "rest-api-tag-from-child-a"
	a.Opts.DatabaseMigratorInfo.ImageTag = "migrator-tag-from-child-a"

	// a later child, with no overrides
	b := NewInstaller()

	assert.Equal(t, originalAgentTag, b.Opts.AgentInfo.ImageTag)
	assert.Equal(t, originalControllerNamespace, b.Opts.ControllerList[0].ImageNamespace)
	require.NotNil(t, b.Opts.ControllerList[1].Enabled)
	assert.True(t, *b.Opts.ControllerList[1].Enabled, "%s was disabled for a later installer", b.Opts.ControllerList[1].Name)
	assert.Equal(t, originalNamespace, b.Opts.Namespace)
	assert.Equal(t, ThreeportRestApi.ImageTag, b.Opts.RestApiInfo.ImageTag)
	assert.Equal(t, DatabaseMigrator.ImageTag, b.Opts.DatabaseMigratorInfo.ImageTag)

	// and the package-level defaults are untouched
	assert.Equal(t, originalAgentTag, ThreeportAgent.ImageTag)
	assert.Equal(t, originalControllerNamespace, ThreeportControllerList[0].ImageNamespace)
	assert.True(t, *ThreeportControllerList[1].Enabled)
}

func TestNewInstaller_AddedControllersDoNotLeakToLaterInstallers(t *testing.T) {
	before := len(NewInstaller().Opts.ControllerList)

	a := NewInstaller(CustomController(&v0.ControlPlaneComponent{Name: "mine"}))
	a.Opts.ControllerList = append(a.Opts.ControllerList, &v0.ControlPlaneComponent{Name: "also-mine"})

	assert.Len(t, a.Opts.ControllerList, before+2)
	assert.Len(t, NewInstaller().Opts.ControllerList, before)
	assert.Len(t, ThreeportControllerList, before)
}

func TestNewInstaller_SharesNothingWithOtherInstallersOrDefaults(t *testing.T) {
	a := NewInstaller()
	b := NewInstaller()

	a.Opts.AdditionalOptions["key"] = "value"
	a.Opts.AdditionalAwsIrsaConditions = append(a.Opts.AdditionalAwsIrsaConditions, "condition")
	assert.Empty(t, b.Opts.AdditionalOptions)
	assert.Empty(t, b.Opts.AdditionalAwsIrsaConditions)

	for i := range a.Opts.ControllerList {
		assert.NotSame(t, ThreeportControllerList[i], a.Opts.ControllerList[i])
		assert.NotSame(t, ThreeportControllerList[i].Enabled, a.Opts.ControllerList[i].Enabled)
		assert.NotSame(t, b.Opts.ControllerList[i], a.Opts.ControllerList[i])
	}
	assert.NotSame(t, ThreeportRestApi, a.Opts.RestApiInfo)
	assert.NotSame(t, ThreeportAgent, a.Opts.AgentInfo)
	assert.NotSame(t, DatabaseMigrator, a.Opts.DatabaseMigratorInfo)
}

func TestNewInstaller_SetAllImagesStayWithTheInstaller(t *testing.T) {
	a := NewInstaller()
	a.SetAllImageRepo("mirror.example.com/threeport")
	a.SetAllImageTags("v9.9.9")

	b := NewInstaller()
	for _, component := range append(b.Opts.ControllerList, b.Opts.RestApiInfo, b.Opts.AgentInfo, b.Opts.DatabaseMigratorInfo) {
		assert.Equal(t, ThreeportImageNamespace, component.ImageNamespace, component.Name)
		assert.NotEqual(t, "v9.9.9", component.ImageTag, component.Name)
	}
}

func TestNewInstaller_ConcurrentInstallersDoNotInterfere(t *testing.T) {
	// run with -race: installers built and overridden from several reconcile
	// workers at once must not write to anything they share
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cpi := NewInstaller(Namespace("child"))
			cpi.Opts.AgentInfo.ImageTag = "tag"
			cpi.Opts.ControllerList[0].ImageNamespace = "registry"
			cpi.Opts.ControllerList = append(cpi.Opts.ControllerList, &v0.ControlPlaneComponent{Name: "mine"})
		}()
	}
	wg.Wait()
}

func TestAllControlPlaneComponents_DoesNotAliasTheDefaults(t *testing.T) {
	all := AllControlPlaneComponents()
	all[0] = &v0.ControlPlaneComponent{Name: "replaced"}

	assert.NotEqual(t, "replaced", ThreeportControllerList[0].Name)
	assert.Len(t, all, len(ThreeportControllerList)+2)
}

func TestCopyComponent_SharesNoMemoryAndDropsIdentity(t *testing.T) {
	id := uint(7)
	enabled := true
	additional := true
	volumes := datatypes.JSON(`{"a":1}`)
	original := &v0.ControlPlaneComponent{
		Common:                 v0.Common{ID: &id},
		ControlPlaneInstanceID: &id,
		Enabled:                &enabled,
		Additional:             &additional,
		Name:                   "mine",
		AdditionalVolumes:      &volumes,
	}

	copied := copyComponent(original)

	assert.Nil(t, copied.ID)
	assert.Nil(t, copied.ControlPlaneInstanceID)
	assert.Equal(t, "mine", copied.Name)

	*copied.Enabled = false
	*copied.Additional = false
	(*copied.AdditionalVolumes)[2] = 'z'
	assert.True(t, *original.Enabled)
	assert.True(t, *original.Additional)
	assert.JSONEq(t, `{"a":1}`, string(*original.AdditionalVolumes))
	assert.Nil(t, copyComponent(nil))
}
