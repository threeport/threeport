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

// Unique-index conflicts arrive as SQLSTATE 23505. The write path turns
// those into a 409 that names the API fields and logs the index name.

const (
	// uniqueViolationCode is SQLSTATE 23505.
	uniqueViolationCode = "23505"

	// ErrMsgUniqueViolation is the 409 body.
	ErrMsgUniqueViolation = "Object conflicts with an existing object"
)

// conflictColumns captures the column list from `Key (a, b)=(...) already exists`.
var conflictColumns = regexp.MustCompile(`^Key \(([^)]+)\)=`)

// schemaCache is gorm's parsed schema, keyed by model type.
var schemaCache = &sync.Map{}

// UniqueConflict is a write rejected by a unique index.
type UniqueConflict struct {
	// The index name, empty if the driver omitted it
	Constraint string

	// The driver detail, including the colliding values
	Detail string

	// API field names for the colliding columns, omitting columns with no field
	Fields []string
}

// Message is the 409 body. It names Fields when they resolved.
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

// Log writes the conflict. Unresolved fields log at error because the
// client message then names nothing to fix.
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

// UniqueViolation returns the unique-index conflict in err, or nil.
// model names the colliding columns; a nil model yields no field names.
func UniqueViolation(err error, model interface{}) *UniqueConflict {
	if err == nil {
		return nil
	}

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

// parseConflictColumns returns the column names in the driver detail.
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

// resolveConflictFields maps database columns to the model's field names.
func resolveConflictFields(model interface{}, columns []string) []string {
	if model == nil || len(columns) == 0 {
		return nil
	}

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

// RespondWriteError returns 409 for a unique-index conflict and 500 otherwise.
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
