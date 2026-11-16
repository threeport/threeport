package v0

import (
	"fmt"
	"reflect"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// This file holds the helpers for writing GORM hooks that stay
// correct across the two call shapes GORM supports. The asymmetry
// between the call shapes is easy to miss, so this block explains the
// mental model in full and the per-function docstrings stay focused on
// what each helper does.
//
// "Hook receiver" here means the *T receiver of the GORM lifecycle
// hook methods (BeforeCreate, BeforeUpdate, BeforeDelete, AfterCreate,
// AfterUpdate, AfterDelete) that the SDK generates on every API type
// into *_validate_gen.go files. GORM calls them automatically when a
// row is being created, updated, or deleted.
//
// The update hooks fire under two call shapes (Updates and Save), and
// the receiver means a different thing in each.
//
// The mental model starts with the generated client functions (T
// stands in for any API type). In the hook column, * stands for both
// the Before and After hook, so *Create means BeforeCreate and
// AfterCreate. The shape column is the receiver layout a hook sees:
// full when the whole object is present (Model == Dest), partial when
// it is a sparse patch over the loaded row (Model != Dest). Note the
// DELETE row: for reconciled types its first GORM call is Updates,
// like PATCH, but the operation later ends in a soft Delete that fires
// the Delete hooks (detailed below the table).
//
//	client   | HTTP   | GORM     | hook     | shape    | effect
//	---------|--------|----------|----------|----------|-------------------------
//	CreateT  | POST   | Create   | *Create  | full     | inserts row
//	ReplaceT | PUT    | Save     | *Update  | full     | writes every column
//	UpdateT  | PATCH  | Updates  | *Update  | partial  | writes only sent fields
//	DeleteT  | DELETE | Updates, | *Update, | partial, | schedules deletion,
//	         |        | Delete   | *Delete  | full     | sets deleted_at
//
// Generated handlers pick the call shape from the HTTP method. The
// UpdateT PATCH handlers use Updates so only fields the client
// actually sent get written. The ReplaceT PUT handlers use Save so
// every column on the bound row lands, including fields the caller
// cleared. DeleteT handlers also use Updates to write only the
// reconciliation flag columns (DeletionScheduled, etc.) without
// touching the rest of the row. GORM fires the Update hooks from
// whatever call shape the caller used.
//
// The hook column shows which generated hooks each call fires. CreateT
// and ReplaceT reach different GORM methods (Create and Save) but
// present the same full-object shape, so they are easy to conflate;
// the shared shape, not a shared method, is what misleads. DeleteT is
// two-phase for reconciled types: the first call schedules deletion
// through Updates and fires the Update hooks, then once the controller
// confirms, the final call soft deletes and fires the Delete hooks.
// That soft delete writes deleted_at with an UPDATE, but GORM keys
// hooks on the operation, so it runs the Delete hooks, not the Update
// hooks.
//
// Under the hood the two call shapes differ in what the hook receiver
// holds. The examples use loaded (a row pulled from the DB), patch
// (the caller's partial payload), and obj (a complete inbound row).
//
//   - Model(&loaded).Updates(&patch): the caller has a payload with
//     only the fields it wants to change; GORM applies those over the
//     loaded row and leaves the rest. Hook receiver is the loaded DB
//     row; the caller's new values live in tx.Statement.Dest.
//
//   - Save(&obj): the caller has a complete object and wants every
//     field persisted. Receiver and tx.Statement.Dest are the same
//     inbound object that already holds the new values.
//
// Create uses the Create method but the Save shape: receiver, Model,
// and Dest all point at the inbound object.
//
// Updates fire from either shape. GORM merges the incoming values
// onto the receiver between the before-update and after-update
// callbacks, so by an after-update hook the receiver already reflects
// the committed result.
//
// To stay correct regardless of shape, hooks read through helpers
// rather than the receiver directly. IsFieldChanged is the high-level
// helper to reach for first; the rest are lower-level building blocks:
//
//   - IsFieldChanged for per-field change detection (works under both
//     PATCH and PUT; handles the DB load internally).
//   - IncomingValues(tx) for the values being written.
//   - LoadObjectFromDB(tx) for a fresh read of the committed row.
//   - IsFullReplace to identify the Save-shape call (PUT) where a nil
//     inbound field means an explicit clear.
//   - IsPartialUpdate to identify the Updates-shape call (PATCH and
//     DELETE) where a nil inbound field means absent-and-unchanged.
//
// Layer choice for the PUT/PATCH naming
//
// The helpers name the distinction at the semantic layer (IsFullReplace
// / IsPartialUpdate). It could have been named at any of three other
// layers:
//
//   layer             | partial detector | on PATCH | on reconciled DELETE | caveat
//   ------------------|------------------|----------|----------------------|-------
//   semantic (chosen) | IsPartialUpdate  | true     | true                 | no
//   GORM method       | IsUpdates        | true     | true                 | no
//   threeport client  | IsUpdate         | true     | false                | yes
//   HTTP verb         | IsPatch          | true     | false                | yes
//
// The alternatives, and why each was passed over:
//
//   - HTTP verb (IsPatch) and threeport client (IsUpdate) look at the
//     request verb and client call, not the GORM call underneath. On a
//     reconciled DELETE, IsPatch checks the verb (DELETE, not PATCH)
//     and IsUpdate checks the client call (DeleteT, not UpdateT), so
//     both return false and miss a call that is partial underneath (a
//     GORM Updates). That miss is the DELETE caveat.
//   - GORM method (IsUpdates) is correct with no caveat, but it names
//     the mechanism, not the intent. A hook only needs "full replace or
//     partial?"; IsUpdates makes the reader map the GORM method onto
//     that answer, where IsPartialUpdate just states it.
//   - IsUpdates (GORM method) and IsUpdate (threeport client) are a
//     single trailing s apart, yet they disagree on the reconciled
//     DELETE: IsUpdates is true (the call is a GORM Updates), IsUpdate
//     is false (the call is DeleteT, not UpdateT). Near-identical names
//     that diverge exactly where it matters are easy to confuse.
//
// The semantic layer carries no caveat and names the call shape
// directly, without HTTP, client, or GORM-method vocabulary, so the
// helpers use it.

// IsFieldChanged reports whether the named field is being modified by
// the current GORM update. Reach for this first for any per-field
// change detection in a hook; it works under both PATCH and PUT call
// shapes.
//
// Under PATCH (Updates), uses tx.Statement.Changed which correctly
// compares the inbound patch against the loaded row (no DB read).
// Under PUT (Save), loads the committed row via LoadObjectFromDB and
// compares the named field by reflection, one DB read per call.
//
// Returns an error if the named field doesn't exist on the type, the
// row has no ID, or the DB load fails. Callers in immutability hooks
// should return the error so GORM aborts the transaction rather than
// proceeding with an undetermined change status.
func IsFieldChanged(tx *gorm.DB, fieldName string) (bool, error) {
	// PATCH fast-path: Statement.Model is the loaded row, Statement.Dest
	// is the inbound patch, and they're different objects. GORM's
	// Statement.Changed compares them directly, so no DB read of our own
	// is needed.
	if IsPartialUpdate(tx) {
		// validate the field exists on the schema; tx.Statement.Changed
		// silently returns false for unknown names, which would let a
		// typo'd field slip through an immutability check on PATCH.
		if tx.Statement.Schema != nil && tx.Statement.Schema.LookUpField(fieldName) == nil {
			return false, fmt.Errorf("IsFieldChanged: field %s not found on %T", fieldName, tx.Statement.Dest)
		}
		return tx.Statement.Changed(fieldName), nil
	}

	// PUT path: Model == Dest, so the inbound IS the receiver. Under
	// Save, tx.Statement.Changed always reports false because there is
	// no separate loaded row to diff against; LoadObjectFromDB pulls
	// the committed row so we can compare manually.
	incoming := tx.Statement.Dest
	pre, err := LoadObjectFromDB(tx)
	if err != nil {
		return false, fmt.Errorf("IsFieldChanged: %w", err)
	}

	// Both pre and incoming are *T; .Elem() unwraps to T so FieldByName
	// can read named struct fields via reflection.
	preVal := reflect.ValueOf(pre).Elem().FieldByName(fieldName)
	newVal := reflect.ValueOf(incoming).Elem().FieldByName(fieldName)
	if !preVal.IsValid() || !newVal.IsValid() {
		return false, fmt.Errorf("IsFieldChanged: field %s not found on %T", fieldName, incoming)
	}

	// DeepEqual handles pointer fields, slices, maps, and primitives
	// uniformly. For *uint FK fields this correctly distinguishes nil
	// from non-nil and any non-equal pair of values.
	return !reflect.DeepEqual(preVal.Interface(), newVal.Interface()), nil
}

// IncomingValues returns the inbound values for the current update.
// Under PATCH this is the patch struct; under PUT it is the inbound
// object (same as the hook receiver). The helper exists so callers
// don't have to remember which call shape they're in.
func IncomingValues(tx *gorm.DB) interface{} {
	return tx.Statement.Dest
}

// LoadObjectFromDB returns a newly-allocated instance of the current
// row's type populated from the database by ID via a fresh session
// that does not inherit the current statement's clauses. Reads
// tx.Statement.Model for the type and ID; under PATCH that is the
// loaded row (which always has the ID), and under PUT that is the
// inbound object (which also has the ID because the caller is
// updating an existing row). Returns an error if Model has no ID or
// the DB load fails.
func LoadObjectFromDB(tx *gorm.DB) (interface{}, error) {
	return loadObjectFromDB(tx, NewCleanSession(tx), "LoadObjectFromDB")
}

// LockObjectInDB is LoadObjectFromDB, and also locks the row until the
// update's transaction ends (SELECT ... FOR UPDATE).
//
// A hook checks a PATCH against the fields it doesn't write with this. The
// receiver holds the row as the handler loaded it before the transaction
// began, and another update may have changed those fields since. The row
// read here is the one the update writes over, and the lock makes a
// concurrent update of the same row wait until this one commits, so that
// update's hooks read the result.
func LockObjectInDB(tx *gorm.DB) (interface{}, error) {
	return loadObjectFromDB(
		tx,
		NewCleanSession(tx).Clauses(clause.Locking{Strength: "UPDATE"}),
		"LockObjectInDB",
	)
}

// loadObjectFromDB loads the row tx.Statement.Model names through session.
func loadObjectFromDB(tx, session *gorm.DB, caller string) (interface{}, error) {
	model := tx.Statement.Model
	id := util.ObjectID(model)
	if id == nil {
		return nil, fmt.Errorf("%s: %T has no ID", caller, model)
	}
	loaded := reflect.New(reflect.TypeOf(model).Elem()).Interface()
	if err := session.First(loaded, *id).Error; err != nil {
		return nil, fmt.Errorf(
			"failed to load %s/%d from database: %w",
			util.ObjectTypeName(model), *id, err,
		)
	}
	return loaded, nil
}

// IsFullReplace reports whether the current update is a PUT (Save,
// where Model == Dest) rather than a PATCH or reconciled DELETE
// flag-write (Updates, where Model != Dest). The distinction matters
// when a nil inbound field means an explicit clear under PUT but
// absent-and-unchanged under PATCH.
func IsFullReplace(tx *gorm.DB) bool {
	return tx.Statement.Model == tx.Statement.Dest
}

// IsPartialUpdate reports whether the current update is the
// Updates-shape call that PATCH and the reconciled DELETE flag-write
// phase use (where Model != Dest).
func IsPartialUpdate(tx *gorm.DB) bool {
	return tx.Statement.Model != tx.Statement.Dest
}
