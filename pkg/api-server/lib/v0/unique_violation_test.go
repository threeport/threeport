package v0

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"
)

// A unique index violation arrives as SQLSTATE 23505 with the colliding columns
// in the error detail, as "Key (name, ip_address)=('demo-a', '10.0.0.42')
// already exists." The detail strings below follow that format. Two negative
// cases carry more than a code mismatch: SQLSTATE 40001, which RetryWrite
// re-runs rather than answering with a 409, and an untyped error holding the
// digits 23505, which classification passes over because it reads the driver's
// typed code alone.

// conflictTestModel is the model conflict columns resolve against. Its
// IPAddress field maps to the column ip_address, so resolution has to report
// the field name the API carries.
type conflictTestModel struct {
	Name      *string
	IPAddress *string
}

// TestUniqueViolationClassifies covers which database errors UniqueViolation
// reports as a conflict and which it passes over.
func TestUniqueViolationClassifies(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		constraint string
		conflict   bool
	}{
		{
			name:       "pg error with 23505 code is a conflict",
			err:        &pgconn.PgError{Code: "23505", ConstraintName: "idx_machine_instance_ip_address"},
			constraint: "idx_machine_instance_ip_address",
			conflict:   true,
		},
		{
			name:       "pg error with 23505 code and no constraint name is a conflict",
			err:        &pgconn.PgError{Code: "23505"},
			constraint: "",
			conflict:   true,
		},
		{
			name:       "wrapped pg error with 23505 code is a conflict",
			err:        fmt.Errorf("persist object: %w", &pgconn.PgError{Code: "23505", ConstraintName: "idx_events_dedup"}),
			constraint: "idx_events_dedup",
			conflict:   true,
		},
		{
			name:     "pg error with 40001 code is not a conflict",
			err:      &pgconn.PgError{Code: "40001"},
			conflict: false,
		},
		{
			name:     "pg error with 23503 code is not a conflict",
			err:      &pgconn.PgError{Code: "23503", ConstraintName: "fk_machine_instance_runtime"},
			conflict: false,
		},
		{
			name:     "untyped error carrying 23505 as data is not a conflict",
			err:      errors.New("failed to provision host-23505"),
			conflict: false,
		},
		{
			name:     "gorm record not found is not a conflict",
			err:      gorm.ErrRecordNotFound,
			conflict: false,
		},
		{
			name:     "nil error is not a conflict",
			err:      nil,
			conflict: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			conflict := UniqueViolation(test.err, &conflictTestModel{})

			assert.Equal(t, test.conflict, conflict != nil)
			if !test.conflict {
				assert.Nil(t, conflict)
				return
			}

			assert.Equal(t, test.constraint, conflict.Constraint)
		})
	}
}

// TestUniqueViolationResolvesFields asserts a conflict detail resolves to the
// model's own field names, and resolves to none when the detail parses no
// columns or no model is given.
func TestUniqueViolationResolvesFields(t *testing.T) {
	tests := []struct {
		name   string
		detail string
		model  interface{}
		fields []string
	}{
		{
			name:   "single column resolves to its field",
			detail: "Key (name)=('demo-a') already exists.",
			model:  &conflictTestModel{},
			fields: []string{"Name"},
		},
		{
			name:   "composite columns resolve in the order reported",
			detail: "Key (name, ip_address)=('demo-a', '10.0.0.42') already exists.",
			model:  &conflictTestModel{},
			fields: []string{"Name", "IPAddress"},
		},
		{
			name:   "acronym column resolves to the field's own casing",
			detail: "Key (ip_address)=('10.0.0.42') already exists.",
			model:  &conflictTestModel{},
			fields: []string{"IPAddress"},
		},
		{
			name:   "column matching no field is dropped",
			detail: "Key (name, deleted_at)=('demo-a', NULL) already exists.",
			model:  &conflictTestModel{},
			fields: []string{"Name"},
		},
		{
			name:   "unrecognized detail resolves no fields",
			detail: "conflicting key detected",
			model:  &conflictTestModel{},
			fields: nil,
		},
		{
			name:   "empty detail resolves no fields",
			detail: "",
			model:  &conflictTestModel{},
			fields: nil,
		},
		{
			name:   "nil model resolves no fields",
			detail: "Key (name)=('demo-a') already exists.",
			model:  nil,
			fields: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := &pgconn.PgError{Code: "23505", Detail: test.detail}

			conflict := UniqueViolation(err, test.model)

			assert.NotNil(t, conflict)
			assert.Equal(t, test.fields, conflict.Fields)

			assert.Equal(t, test.detail, conflict.Detail)
		})
	}
}

// TestUniqueConflictMessageNamesFields asserts the client message lists the
// resolved fields, and drops the list when nothing resolved.
func TestUniqueConflictMessageNamesFields(t *testing.T) {
	tests := []struct {
		name     string
		conflict UniqueConflict
		message  string
	}{
		{
			name:     "one field is named",
			conflict: UniqueConflict{Fields: []string{"Hostname"}},
			message:  "Object conflicts with an existing object on: Hostname",
		},
		{
			name:     "several fields are named in order",
			conflict: UniqueConflict{Fields: []string{"Name", "ApiNamespace"}},
			message:  "Object conflicts with an existing object on: Name, ApiNamespace",
		},
		{
			name:     "no fields falls back to the unqualified text",
			conflict: UniqueConflict{},
			message:  "Object conflicts with an existing object",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.message, test.conflict.Message())
		})
	}
}

// TestUniqueConflictLogRaisesUnresolvedToError asserts a conflict logs at info
// once its fields resolve and at error when they do not.
func TestUniqueConflictLogRaisesUnresolvedToError(t *testing.T) {
	tests := []struct {
		name     string
		conflict UniqueConflict
		level    zapcore.Level
		message  string
	}{
		{
			name: "resolved fields log at info",
			conflict: UniqueConflict{
				Constraint: "idx_machine_runtime_instance_hostname",
				Detail:     "Key (hostname)=('10.0.0.42') already exists.",
				Fields:     []string{"Hostname"},
			},
			level:   zapcore.InfoLevel,
			message: "write rejected by unique index",
		},
		{
			name: "unresolved fields log at error",
			conflict: UniqueConflict{
				Constraint: "idx_machine_runtime_instance_hostname",
				Detail:     "conflicting key detected",
			},
			level:   zapcore.ErrorLevel,
			message: "unique index rejected a write and the fields behind it did not resolve",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			core, recorded := observer.New(zapcore.DebugLevel)

			test.conflict.Log(zap.New(core))

			entries := recorded.All()
			assert.Len(t, entries, 1)
			assert.Equal(t, test.level, entries[0].Level)
			assert.Equal(t, test.message, entries[0].Message)
		})
	}
}

// TestUniqueConflictLogAcceptsNilLogger asserts Log returns without panicking
// when a caller hands it no logger.
func TestUniqueConflictLogAcceptsNilLogger(t *testing.T) {
	conflict := UniqueConflict{Fields: []string{"Hostname"}}

	assert.NotPanics(t, func() { conflict.Log(nil) })
}
