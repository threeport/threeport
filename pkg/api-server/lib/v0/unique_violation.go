package v0

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
	"gorm.io/gorm/schema"
)

// A write against a unique index comes back with SQLSTATE 23505, the index in
// the error's constraint, and the conflicting columns and values in its detail
// text, shaped as Key (name, ip_address)=('demo-a', '10.0.0.42') already
// exists. A duplicate is the client's mistake, so the write path answers 409
// naming the API fields behind those columns and keeps the index name out of
// the response. The index name goes to the log on every conflict, and the
// detail text, values included, goes with it when no field resolves.

const (
	// uniqueViolationCode is the SQLSTATE for a unique violation, the code
	// PostgreSQL and CockroachDB both return.
	uniqueViolationCode = "23505"

	// ErrMsgUniqueViolation is the text a conflict response carries, with the
	// fields appended when they resolve.
	ErrMsgUniqueViolation = "Object conflicts with an existing object"
)

// conflictColumns pulls the column list out of the driver's detail text. The
// match stops at the closing parenthesis, so the values that follow never enter
// the capture group and cannot reach the response.
var conflictColumns = regexp.MustCompile(`^Key \(([^)]+)\)=`)

// schemaCache holds one parsed schema per model type, so naming the fields
// behind a conflict reflects over a type once per process.
var schemaCache = &sync.Map{}

// UniqueConflict is a write the database rejected because it duplicates a row
// a unique index already covers.
type UniqueConflict struct {
	// The name of the index that rejected the write, empty when the driver
	// reported none
	Constraint string

	// The driver's detail text, which carries the conflicting values
	Detail string

	// The API field names the conflicting columns resolved to, in the order the
	// database reported them, dropping any column the model has no field for
	Fields []string
}

// Message returns the conflict text for the response body, naming the fields in
// Fields when there are any. A field name is safe to publish, unlike an index
// name, since it is already part of the API the client wrote against.
func (conflict *UniqueConflict) Message() string {
	if len(conflict.Fields) == 0 {
		return ErrMsgUniqueViolation
	}

	return fmt.Sprintf(
		"%s on: %s",
		ErrMsgUniqueViolation,
		strings.Join(conflict.Fields, ", "),
	)
}

// Log records the conflict, at error level when no field resolved, since the
// message the caller then sends names nothing to correct.
func (conflict *UniqueConflict) Log(logger *zap.Logger) {
	if logger == nil {
		return
	}

	if len(conflict.Fields) == 0 {
		logger.Error(
			"unique index rejected a write and the fields behind it did not resolve",
			zap.String("constraint", conflict.Constraint),
			zap.String("detail", conflict.Detail),
		)
		return
	}

	logger.Info(
		"write rejected by unique index",
		zap.String("constraint", conflict.Constraint),
		zap.Strings("fields", conflict.Fields),
	)
}

// UniqueViolation returns the conflict behind err, or nil when err is not a
// unique index rejecting a write. model is the type the write targeted, used to
// name the conflicting columns; a nil model yields a conflict naming none.
func UniqueViolation(err error, model interface{}) *UniqueConflict {
	if err == nil {
		return nil
	}

	// match the driver's typed code so digits in other error text never count
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != uniqueViolationCode {
		return nil
	}

	return &UniqueConflict{
		Constraint: pgErr.ConstraintName,
		Detail:     pgErr.Detail,
		Fields:     resolveConflictFields(model, parseConflictColumns(pgErr.Detail)),
	}
}

// parseConflictColumns returns the column names the driver's detail text
// reports, or nil when the text is not in that form.
func parseConflictColumns(detail string) []string {
	match := conflictColumns.FindStringSubmatch(detail)
	if match == nil {
		return nil
	}

	columns := strings.Split(match[1], ",")
	for i, column := range columns {
		columns[i] = strings.TrimSpace(column)
	}

	return columns
}

// resolveConflictFields maps database column names onto the model's field
// names, dropping any column the model has no field for.
func resolveConflictFields(model interface{}, columns []string) []string {
	if model == nil || len(columns) == 0 {
		return nil
	}

	// read the names off the schema rather than undo the column casing by rule,
	// which would not put an acronym back together
	modelSchema, err := schema.Parse(model, schemaCache, schema.NamingStrategy{})
	if err != nil {
		return nil
	}

	var fields []string
	for _, column := range columns {
		field, ok := modelSchema.FieldsByDBName[column]
		if !ok {
			continue
		}
		fields = append(fields, field.Name)
	}

	return fields
}

// RespondWriteError answers a failed write: 409 when a unique index rejected
// it, naming the resolved fields, and 500 for every other error. model is a
// zero value of the object being written.
func RespondWriteError(
	c echo.Context,
	logger *zap.Logger,
	err error,
	model interface{},
	fullyQualifiedType string,
) error {
	if conflict := UniqueViolation(err, model); conflict != nil {
		conflict.Log(logger)
		return ResponseStatus409(c, nil, errors.New(conflict.Message()), fullyQualifiedType)
	}

	return ResponseStatus500(c, nil, err, fullyQualifiedType)
}
