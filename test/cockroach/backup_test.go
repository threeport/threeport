package cockroach

import (
	"strings"
	"testing"

	installer "github.com/threeport/threeport/pkg/threeport-installer/v0"
)

// testBackupURI is where these tests back up to: the container's own disk
// stands in for the GCS bucket a production control plane uses.
const testBackupURI = "nodelocal://1/threeport_api"

// TestTheBackupScheduleRunsAfterTheInitScript covers the backup schedule as
// the database migrator creates it: on its own, after the init script, and
// again on every start of the API server without adding a second schedule.
// CockroachDB checks the backup target when the schedule is created, which
// is why the two cannot run as one script.
func TestTheBackupScheduleRunsAfterTheInitScript(t *testing.T) {
	root, err := open(testPort, "defaultdb")
	if err != nil {
		t.Fatalf("open defaultdb: %v", err)
	}
	schedule := installer.DatabaseBackupScheduleSQL(testBackupURI)

	for start := 0; start < 2; start++ {
		if err := root.Exec(schedule).Error; err != nil {
			t.Fatalf("start %d: create the backup schedule: %v", start, err)
		}
	}

	// a schedule is a full and an incremental schedule under one label
	var recurrences []string
	if err := root.Raw(
		"SELECT recurrence FROM [SHOW SCHEDULES] WHERE label = ? ORDER BY recurrence",
		installer.DatabaseBackupScheduleLabel,
	).Scan(&recurrences).Error; err != nil {
		t.Fatalf("show schedules: %v", err)
	}
	if strings.Join(recurrences, ",") != "@daily,@hourly" {
		t.Fatalf("schedules = %v, want one daily full and one hourly incremental", recurrences)
	}

	// the first full backup is due as soon as the schedule exists, rather
	// than at the next midnight; the scheduler picks it up within minutes
	var due bool
	if err := root.Raw(
		"SELECT next_run <= now() FROM [SHOW SCHEDULES] WHERE label = ? AND recurrence = ?",
		installer.DatabaseBackupScheduleLabel, installer.DatabaseFullBackupRecurrence,
	).Scan(&due).Error; err != nil {
		t.Fatalf("show schedules: %v", err)
	}
	if !due {
		t.Error("the first full backup is not due yet; it waits for the next scheduled run")
	}
}

// TestTheBackupScheduleFailsInTheInitScript keeps the reason the schedule
// runs on its own: in one script with the statement that creates the
// database, the schedule fails and takes the whole script down with it.
func TestTheBackupScheduleFailsInTheInitScript(t *testing.T) {
	root, err := open(testPort, "defaultdb")
	if err != nil {
		t.Fatalf("open defaultdb: %v", err)
	}

	script := "CREATE DATABASE IF NOT EXISTS backup_target_check;\n" +
		strings.ReplaceAll(
			strings.ReplaceAll(installer.DatabaseBackupScheduleSQL("nodelocal://1/check"), installer.DatabaseName, "backup_target_check"),
			installer.DatabaseBackupScheduleLabel, "backup_target_check",
		)
	if err := root.Exec(script).Error; err == nil {
		t.Fatal("the schedule succeeded in the same script as the database it backs up; it could run in the init script")
	}
}
