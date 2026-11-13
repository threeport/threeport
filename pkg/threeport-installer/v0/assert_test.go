package v0

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	api "github.com/threeport/threeport/pkg/api/v0"
)

// TestRestoredDatabaseCheckRequiresTheUserTheDatabaseAndTheSchedule checks the database statements.
func TestRestoredDatabaseCheckRequiresTheUserTheDatabaseAndTheSchedule(t *testing.T) {
	// require the user, the database, and the schedule when a bucket is set
	withSchedule := strings.Join(restoredDatabaseCheckSQL(true), "\n")
	require.Contains(t, withSchedule, "threeport user must exist")
	require.Contains(t, withSchedule, "threeport_api database must exist")
	require.Contains(t, withSchedule, "threeport_api_backup schedule must exist")
	require.Contains(t, withSchedule, DatabaseBackupScheduleLabel)
	require.Contains(t, withSchedule, ">= 1")

	// omit the schedule check when no bucket is set
	withoutSchedule := strings.Join(restoredDatabaseCheckSQL(false), "\n")
	require.NotContains(t, withoutSchedule, "schedule must exist")
	require.NotContains(t, withoutSchedule, ">= 1")
	require.Contains(t, withoutSchedule, "threeport user must exist")
}

// TestControllerSetDifferenceNamesMissingAndExtra checks both sides of the controller set.
func TestControllerSetDifferenceNamesMissingAndExtra(t *testing.T) {
	// accept an exact match
	require.NoError(t, controllerSetDifference(
		[]string{"control-plane"},
		[]string{"control-plane"},
	))

	// name an installed controller the selection did not ask for
	extra := controllerSetDifference(
		[]string{"control-plane"},
		[]string{"aws", "control-plane"},
	)
	require.Error(t, extra)
	require.Contains(t, extra.Error(), "extra [aws]")

	// name a selected controller that is not installed
	missing := controllerSetDifference(
		[]string{"control-plane", "gateway"},
		[]string{"control-plane"},
	)
	require.Error(t, missing)
	require.Contains(t, missing.Error(), "missing [gateway]")
}

// TestEnabledControllerNamesTreatsANilFlagAsInstalled checks the default controller flag.
func TestEnabledControllerNamesTreatsANilFlagAsInstalled(t *testing.T) {
	// keep a nil flag and drop an explicit false
	off := false
	names := enabledControllerNames([]*api.ControlPlaneComponent{
		{Name: "control-plane"},
		{Name: "aws", Enabled: &off},
	})
	require.Equal(t, []string{"control-plane"}, names)
}
