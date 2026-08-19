package v0

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// hostnameIndexName is the partial unique index that holds machine runtime
// instances to one live row per hostname.
const hostnameIndexName = "idx_machine_runtime_instance_hostname"

// hostnameDryRunDB opens a gorm handle that builds statements without running
// them, so the schema declared by the struct tags is exercised against the
// postgres grammar CockroachDB speaks. SkipDefaultTransaction matters here:
// the wrapping transaction gorm opens around a write dials the database,
// which dry run alone does not prevent.
func hostnameDryRunDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(
		postgres.New(postgres.Config{
			DSN:                  "postgres://u:p@127.0.0.1:26257/threeport_api?sslmode=disable",
			PreferSimpleProtocol: true,
		}),
		&gorm.Config{DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true},
	)
	require.NoError(t, err, "opening the dry-run handle")
	return db
}

// hostnameIndexSQL builds the CREATE INDEX statement for the named index and
// returns it without touching a database.
func hostnameIndexSQL(t *testing.T, db *gorm.DB, name string) string {
	t.Helper()
	var captured string
	require.NoError(
		t,
		db.Callback().Raw().After("gorm:raw").Register(
			"test:capture_index_sql",
			func(tx *gorm.DB) { captured = tx.Statement.SQL.String() },
		),
		"registering the statement capture",
	)
	require.NoError(t, db.Migrator().CreateIndex(&MachineRuntimeInstance{}, name), "building the index statement")
	require.NotEmpty(t, captured, "the migrator built no statement for %s", name)
	return captured
}

// TestMachineRuntimeInstanceHostnameUniqueIndex pins the write contract: the
// hostname column carries a unique index, so two machine runtime instances
// cannot claim the same machine and reconcile against it at the same time.
func TestMachineRuntimeInstanceHostnameUniqueIndex(t *testing.T) {
	sql := hostnameIndexSQL(t, hostnameDryRunDB(t), hostnameIndexName)

	assert.Contains(t, strings.ToUpper(sql), "CREATE UNIQUE INDEX",
		"the hostname index must be unique to keep two records off one machine:\n%s", sql)
	assert.Contains(t, sql, `"hostname"`,
		"the index must cover the hostname column:\n%s", sql)
	assert.Contains(t, sql, hostnameIndexName,
		"the index must carry its declared name so a violation names it:\n%s", sql)
}

// TestMachineRuntimeInstanceHostnameIndexSkipsDeleted pins the partial
// predicate. Machine runtime instances use gorm soft delete, so a deleted row
// keeps its hostname value; the predicate holds those rows out of the unique
// slot and lets a fresh instance take the hostname over.
func TestMachineRuntimeInstanceHostnameIndexSkipsDeleted(t *testing.T) {
	sql := hostnameIndexSQL(t, hostnameDryRunDB(t), hostnameIndexName)

	upper := strings.ToUpper(sql)
	assert.Contains(t, upper, "WHERE",
		"the index must be partial:\n%s", sql)
	assert.Contains(t, upper, "DELETED_AT IS NULL",
		"soft-deleted rows must fall outside the unique slot:\n%s", sql)
}
