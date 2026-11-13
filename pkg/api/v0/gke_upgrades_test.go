package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestAGkeReleaseChannelAndMaintenanceWindowAreChecked covers the release
// channels and maintenance windows a GKE definition can ask for.
func TestAGkeReleaseChannelAndMaintenanceWindowAreChecked(t *testing.T) {
	require.Contains(t, GkeReleaseChannels, DefaultGkeReleaseChannel)
	require.Contains(t, GkeReleaseChannels, ProductionGkeReleaseChannel)
	require.NoError(t, ValidateGkeUpgradeSettings(
		util.Ptr(DefaultGkeReleaseChannel),
		util.Ptr(DefaultGkeMaintenanceWindowDays),
		util.Ptr(DefaultGkeMaintenanceWindowStart),
		util.Ptr(DefaultGkeMaintenanceWindowHours),
	), "the defaults")
	assert.NoError(t, ValidateGkeUpgradeSettings(nil, nil, nil, nil), "all unset")

	for _, channel := range []string{"REGULAR", "STABLE"} {
		assert.NoError(t, ValidateGkeUpgradeSettings(util.Ptr(channel), nil, nil, nil), channel)
	}
	for _, channel := range []string{"RAPID", "EXTENDED", "UNSPECIFIED", "regular", ""} {
		assert.ErrorContains(t, ValidateGkeUpgradeSettings(util.Ptr(channel), nil, nil, nil),
			"is not supported: use one of REGULAR, STABLE", "%q", channel)
	}

	for _, days := range []string{"SU", "MO,WE,FR", "MO,TU,WE,TH,FR,SA,SU"} {
		assert.NoError(t, ValidateGkeUpgradeSettings(nil, util.Ptr(days), nil, nil), days)
	}
	for _, days := range []string{"", "SA,SA", "SA, SU", "sa,su", "SAT", "SA,", "SA;SU"} {
		assert.ErrorContains(t, ValidateGkeUpgradeSettings(nil, util.Ptr(days), nil, nil),
			"is not a list of distinct days", "%q", days)
	}

	for _, start := range []string{"00:00", "04:00", "23:30"} {
		assert.NoError(t, ValidateGkeUpgradeSettings(nil, nil, util.Ptr(start), nil), start)
	}
	for _, start := range []string{"", "4:00", "24:00", "04:60", "04:00Z", "04:00:00", "4am"} {
		assert.ErrorContains(t, ValidateGkeUpgradeSettings(nil, nil, util.Ptr(start), nil),
			"is not a time of day in UTC", "%q", start)
	}

	for _, hours := range []int{4, 8, 24} {
		assert.NoError(t, ValidateGkeUpgradeSettings(nil, nil, nil, util.Ptr(hours)), "%d", hours)
	}
	for _, hours := range []int{0, 3, 25, -4} {
		assert.ErrorContains(t, ValidateGkeUpgradeSettings(nil, nil, nil, util.Ptr(hours)),
			"GKE upgrades only in a window of 4 to 24 hours", "%d", hours)
	}

	assert.Equal(t, "REGULAR", GkeReleaseChannelOrDefault(nil))
	assert.Equal(t, "STABLE", GkeReleaseChannelOrDefault(util.Ptr("STABLE")))
	assert.Equal(t, "SA,SU", GkeMaintenanceWindowDaysOrDefault(nil))
	assert.Equal(t, "MO", GkeMaintenanceWindowDaysOrDefault(util.Ptr("MO")))
	assert.Equal(t, "04:00", GkeMaintenanceWindowStartOrDefault(util.Ptr("")))
	assert.Equal(t, "22:00", GkeMaintenanceWindowStartOrDefault(util.Ptr("22:00")))
	assert.Equal(t, 8, GkeMaintenanceWindowHoursOrDefault(nil))
	assert.Equal(t, 6, GkeMaintenanceWindowHoursOrDefault(util.Ptr(6)))
}

// TestAGkeDefinitionsReleaseChannelAndMaintenanceWindowAreKept covers the
// API: a GKE definition is checked on create, and refused a change to its
// release channel or maintenance window on either update shape.
func TestAGkeDefinitionsReleaseChannelAndMaintenanceWindowAreKept(t *testing.T) {
	db := setupKubernetesApiAccessDB(t)
	build := func(name string) *GcpGkeKubernetesRuntimeDefinition {
		return &GcpGkeKubernetesRuntimeDefinition{
			Definition: Definition{Name: util.Ptr(name)}, ZoneCount: util.Ptr(3), DefaultNodeGroupInstanceType: util.Ptr("e2-medium"),
			DefaultNodeGroupInitialSize: util.Ptr(1), DefaultNodeGroupMinimumSize: util.Ptr(1), DefaultNodeGroupMaximumSize: util.Ptr(3),
			KubernetesRuntimeDefinitionID: util.Ptr(uint(1)),
			ReleaseChannel:                util.Ptr("STABLE"),
			MaintenanceWindowDays:         util.Ptr("SU"),
			MaintenanceWindowStart:        util.Ptr("22:00"),
			MaintenanceWindowHours:        util.Ptr(6),
		}
	}

	for field, spoil := range map[string]func(*GcpGkeKubernetesRuntimeDefinition){
		"ReleaseChannel":         func(g *GcpGkeKubernetesRuntimeDefinition) { g.ReleaseChannel = util.Ptr("RAPID") },
		"MaintenanceWindowDays":  func(g *GcpGkeKubernetesRuntimeDefinition) { g.MaintenanceWindowDays = util.Ptr("SUN") },
		"MaintenanceWindowStart": func(g *GcpGkeKubernetesRuntimeDefinition) { g.MaintenanceWindowStart = util.Ptr("25:00") },
		"MaintenanceWindowHours": func(g *GcpGkeKubernetesRuntimeDefinition) { g.MaintenanceWindowHours = util.Ptr(2) },
	} {
		refused := build("refused-" + field)
		spoil(refused)
		assert.ErrorContains(t, db.Create(refused).Error, field, "a create")
	}

	created := build("gke")
	require.NoError(t, db.Create(created).Error)
	load := func() *GcpGkeKubernetesRuntimeDefinition {
		var loaded GcpGkeKubernetesRuntimeDefinition
		require.NoError(t, db.First(&loaded, *created.ID).Error)
		return &loaded
	}

	for field, change := range map[string]func(*GcpGkeKubernetesRuntimeDefinition){
		"ReleaseChannel":         func(g *GcpGkeKubernetesRuntimeDefinition) { g.ReleaseChannel = util.Ptr("REGULAR") },
		"MaintenanceWindowDays":  func(g *GcpGkeKubernetesRuntimeDefinition) { g.MaintenanceWindowDays = util.Ptr("SA") },
		"MaintenanceWindowStart": func(g *GcpGkeKubernetesRuntimeDefinition) { g.MaintenanceWindowStart = util.Ptr("02:00") },
		"MaintenanceWindowHours": func(g *GcpGkeKubernetesRuntimeDefinition) { g.MaintenanceWindowHours = util.Ptr(8) },
	} {
		patch := &GcpGkeKubernetesRuntimeDefinition{}
		change(patch)
		assert.ErrorContains(t, db.Session(&gorm.Session{}).Model(load()).Updates(patch).Error,
			ErrMsgGkeUpgradeSettingsImmutable, "a patch of %s", field)

		replaced := load()
		change(replaced)
		assert.ErrorContains(t, db.Session(&gorm.Session{}).Save(replaced).Error,
			ErrMsgGkeUpgradeSettingsImmutable, "a replace of %s", field)
	}

	require.NoError(t, db.Session(&gorm.Session{}).Model(load()).Updates(build("gke")).Error,
		"sending the settings it holds is not a change")
	require.NoError(t, db.Session(&gorm.Session{}).Save(load()).Error, "nor is a replace that keeps them")
	require.NoError(t, db.Session(&gorm.Session{}).Model(load()).Updates(
		&GcpGkeKubernetesRuntimeDefinition{DefaultNodeGroupMaximumSize: util.Ptr(5)},
	).Error)

	stored := load()
	assert.Equal(t, "STABLE", *stored.ReleaseChannel)
	assert.Equal(t, "SU", *stored.MaintenanceWindowDays)
	assert.Equal(t, "22:00", *stored.MaintenanceWindowStart)
	assert.Equal(t, 6, *stored.MaintenanceWindowHours)
}
