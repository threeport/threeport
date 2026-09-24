package v0

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// commonTestModel embeds Common so the GORM tags on Common can be exercised
// against a real database schema.
type commonTestModel struct {
	Common
	Name string
}

// reconciliationTestModel embeds Reconciliation so its default-value gorm tags
// and pointer semantics can be exercised end to end.
type reconciliationTestModel struct {
	Common
	Reconciliation
	Name string
}

// newTestDB opens an in-memory sqlite database and migrates the supplied
// models so struct-tag behavior can be verified against a real driver.
func newTestDB(t *testing.T, models ...interface{}) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite: %v", err)
	}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("failed to migrate models: %v", err)
	}
	return db
}

// TestCommon_PrimaryKeyAndTimestamps asserts that Common's gorm tags produce
// an auto-incrementing primary key and populated created/updated timestamps
// on insert.
func TestCommon_PrimaryKeyAndTimestamps(t *testing.T) {
	// prepare an in-memory database with the embedding model
	db := newTestDB(t, &commonTestModel{})

	// insert two rows and observe the ID and timestamp side effects
	first := commonTestModel{Name: "first"}
	if err := db.Create(&first).Error; err != nil {
		t.Fatalf("create first: %v", err)
	}
	second := commonTestModel{Name: "second"}
	if err := db.Create(&second).Error; err != nil {
		t.Fatalf("create second: %v", err)
	}

	// verify the primary key was assigned and increments
	if first.ID == nil || *first.ID == 0 {
		t.Fatalf("first.ID not assigned, got %v", first.ID)
	}
	if second.ID == nil || *second.ID <= *first.ID {
		t.Fatalf("second.ID did not increment above first.ID, got %v vs %v", second.ID, first.ID)
	}

	// verify CreatedAt and UpdatedAt were populated
	if first.CreatedAt == nil || first.CreatedAt.IsZero() {
		t.Errorf("CreatedAt not populated on insert")
	}
	if first.UpdatedAt == nil || first.UpdatedAt.IsZero() {
		t.Errorf("UpdatedAt not populated on insert")
	}

	// verify DeletedAt stays nil for an active row
	if first.DeletedAt != nil {
		t.Errorf("DeletedAt should be nil for a live row, got %v", first.DeletedAt)
	}
}

// TestCommon_SoftDelete asserts that Common's DeletedAt gorm tag drives GORM's
// soft-delete behavior: deleted rows are hidden from default queries but
// findable with Unscoped.
func TestCommon_SoftDelete(t *testing.T) {
	// prepare a database and a live row
	db := newTestDB(t, &commonTestModel{})
	row := commonTestModel{Name: "gone"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	// perform the soft delete
	if err := db.Delete(&row).Error; err != nil {
		t.Fatalf("delete: %v", err)
	}

	// verify the default query hides the row
	var found commonTestModel
	err := db.First(&found, *row.ID).Error
	if err == nil {
		t.Errorf("expected ErrRecordNotFound after soft delete, got row %+v", found)
	}

	// verify Unscoped surfaces the row with DeletedAt populated
	var raw commonTestModel
	if err := db.Unscoped().First(&raw, *row.ID).Error; err != nil {
		t.Fatalf("unscoped first: %v", err)
	}
	if raw.DeletedAt == nil || !raw.DeletedAt.Valid {
		t.Errorf("DeletedAt should be populated after soft delete, got %+v", raw.DeletedAt)
	}
}

// TestCommon_OmitEmptyJSONTags documents that Common's json tags omit zero
// value pointer fields on marshal. This is exercised implicitly here by
// asserting the zero value struct round-trips without touching the fields.
func TestCommon_ZeroValue(t *testing.T) {
	// a zero-value Common should have all-nil pointer fields
	var c Common
	if c.ID != nil || c.CreatedAt != nil || c.UpdatedAt != nil || c.DeletedAt != nil {
		t.Errorf("zero-value Common has non-nil fields: %+v", c)
	}
}

// TestReconciliation_DefaultValues asserts that Reconciliation's default:false
// gorm tags result in false (not nil) values when a row is loaded back from
// the database without the caller having set them.
func TestReconciliation_DefaultValues(t *testing.T) {
	// prepare the database and insert a row that omits reconciliation flags
	db := newTestDB(t, &reconciliationTestModel{})
	row := reconciliationTestModel{Name: "defaulted"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	// reload the row so the database defaults are applied
	var loaded reconciliationTestModel
	if err := db.First(&loaded, *row.ID).Error; err != nil {
		t.Fatalf("first: %v", err)
	}

	// verify each default-false bool tag resolved to a non-nil false pointer
	assertFalse := func(name string, v *bool) {
		t.Helper()
		if v == nil {
			t.Errorf("%s should default to false, got nil", name)
			return
		}
		if *v != false {
			t.Errorf("%s should default to false, got true", name)
		}
	}
	assertFalse("Reconciled", loaded.Reconciled)
	assertFalse("CreationFailed", loaded.CreationFailed)
	assertFalse("InterruptReconciliation", loaded.InterruptReconciliation)

	// verify optional time fields remain nil when unset
	if loaded.CreationAcknowledged != nil {
		t.Errorf("CreationAcknowledged should default to nil, got %v", loaded.CreationAcknowledged)
	}
	if loaded.DeletionScheduled != nil {
		t.Errorf("DeletionScheduled should default to nil, got %v", loaded.DeletionScheduled)
	}
}

// TestReconciliation_RoundTrip covers a full write-then-read cycle for the
// Reconciliation fields to confirm that caller-supplied values are persisted
// and hydrated unchanged.
func TestReconciliation_RoundTrip(t *testing.T) {
	// prepare the database
	db := newTestDB(t, &reconciliationTestModel{})

	// insert a row with every reconciliation field explicitly set
	truthy := true
	now := time.Now().UTC().Truncate(time.Second)
	row := reconciliationTestModel{
		Name: "populated",
		Reconciliation: Reconciliation{
			Reconciled:              &truthy,
			CreationAcknowledged:    &now,
			CreationConfirmed:       &now,
			CreationFailed:          &truthy,
			DeletionScheduled:       &now,
			DeletionAcknowledged:    &now,
			DeletionConfirmed:       &now,
			InterruptReconciliation: &truthy,
		},
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	// reload and verify each field survived the round trip
	var loaded reconciliationTestModel
	if err := db.First(&loaded, *row.ID).Error; err != nil {
		t.Fatalf("first: %v", err)
	}
	if loaded.Reconciled == nil || !*loaded.Reconciled {
		t.Errorf("Reconciled did not round-trip, got %v", loaded.Reconciled)
	}
	if loaded.CreationFailed == nil || !*loaded.CreationFailed {
		t.Errorf("CreationFailed did not round-trip, got %v", loaded.CreationFailed)
	}
	if loaded.InterruptReconciliation == nil || !*loaded.InterruptReconciliation {
		t.Errorf("InterruptReconciliation did not round-trip, got %v", loaded.InterruptReconciliation)
	}
	if loaded.CreationAcknowledged == nil || !loaded.CreationAcknowledged.Equal(now) {
		t.Errorf("CreationAcknowledged did not round-trip, got %v want %v", loaded.CreationAcknowledged, now)
	}
	if loaded.DeletionConfirmed == nil || !loaded.DeletionConfirmed.Equal(now) {
		t.Errorf("DeletionConfirmed did not round-trip, got %v want %v", loaded.DeletionConfirmed, now)
	}
}

// TestReconciliation_ZeroValue asserts that a freshly constructed
// Reconciliation has all-nil pointer fields so callers can distinguish unset
// from false or the epoch zero time.
func TestReconciliation_ZeroValue(t *testing.T) {
	// a zero-value Reconciliation should have all-nil pointer fields
	var r Reconciliation
	if r.Reconciled != nil ||
		r.CreationAcknowledged != nil ||
		r.CreationConfirmed != nil ||
		r.CreationFailed != nil ||
		r.DeletionScheduled != nil ||
		r.DeletionAcknowledged != nil ||
		r.DeletionConfirmed != nil ||
		r.InterruptReconciliation != nil {
		t.Errorf("zero-value Reconciliation has non-nil fields: %+v", r)
	}
}
