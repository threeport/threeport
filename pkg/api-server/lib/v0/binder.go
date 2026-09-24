package v0

import (
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

// reservedQueryParams are keys the api-server layer consumes directly
// for pagination and query scopes rather than binding onto a filter
// struct. The query binder treats them as known so requests carrying
// them are not rejected as unknown-key errors.
var reservedQueryParams = map[string]bool{
	QueryParamQueryId:        true,
	QueryParamCursor:         true,
	QueryParamLimit:          true,
	QueryParamIncludeDeleted: true,
	// the ids scope filter restricts a list to a set of row ids. It is
	// spelled as a literal because no constant names it in this package.
	"ids": true,
}

// QueryKeyExtender lets a bound type declare query keys a handler
// consumes directly (via `QueryParam()`) rather than binding onto one of
// the type's fields. The binder treats the declared keys as known so a
// well-formed request carrying them is not rejected as unknown, while a
// genuinely unknown key on the same endpoint is still rejected. Keys are
// compared case-insensitively, matching the field-name derivation.
type QueryKeyExtender interface {
	ExtraQueryKeys() []string
}

// QueryBinder overrides echo's default binder so api types don't need
// `query:"..."` struct tags. Each settable struct field is bound from
// the query param keyed by strings.ToLower of the field name. A field
// named KubernetesWorkloadInstanceID binds the
// kubernetesworkloadinstanceid param. Codegen rejects any explicit
// `query:` tag on api types, so the lowercased-field-name derivation is
// the only path here.
//
// Path params and body binding fall through to echo.DefaultBinder.
// Query binding fires on GET, DELETE, and HEAD, matching the default
// binder's method branching.
//
// Behavior we don't carry over from the default binder, because no
// current api type needs it:
//   - Repeated params like ?foo=1&foo=2. We take raw[0] and drop the
//     rest. The default binder reads a slice via the query tag.
//   - time.Time, time.Duration, custom UnmarshalParam. Only string,
//     bool, int*, uint*, and float* are supported. Other kinds return
//     an error rather than skip silently.
//   - `form:"..."` tags on GET-like methods. Threeport doesn't use
//     form encoding for read endpoints.
//   - Non-anonymous embedded fields. Only anonymous embeds (Common,
//     Definition, Instance, Reconciliation) are recursed into.
//
// References:
//
//	echo source:  https://github.com/labstack/echo/blob/master/bind.go
//	echo binding: https://echo.labstack.com/docs/binding
type QueryBinder struct {
	fallback echo.DefaultBinder
}

// NewQueryBinder returns a binder ready to register via
// e.Binder = NewQueryBinder().
func NewQueryBinder() *QueryBinder { return &QueryBinder{} }

// Bind runs the three-stage dispatch: path params, then query or body
// depending on HTTP method.
func (b *QueryBinder) Bind(i interface{}, c echo.Context) error {
	// stage 1: path params via the default binder
	if err := b.fallback.BindPathParams(c, i); err != nil {
		return err
	}

	// stage 2: read methods take input from the URL query, write
	// methods from the body. Matches echo.DefaultBinder.Bind.
	method := c.Request().Method
	if method == http.MethodGet || method == http.MethodDelete || method == http.MethodHead {
		return b.bindQueryParams(c.QueryParams(), i)
	}
	return b.fallback.BindBody(c, i)
}

// bindQueryParams unwraps the target pointer and dispatches to the
// per-field walk when it points at a struct. Unknown query keys are
// rejected upfront so a typo or filter against a nonexistent field
// surfaces as a 400 instead of silently returning unfiltered results.
func (b *QueryBinder) bindQueryParams(qp url.Values, i interface{}) error {
	// no params in the URL means no bind work to do
	if len(qp) == 0 {
		return nil
	}

	// echo always passes a non-nil pointer; surface a misuse loudly
	// rather than silently no-op
	v := reflect.ValueOf(i)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return fmt.Errorf("QueryBinder: target must be a non-nil pointer, got %T", i)
	}
	v = v.Elem()

	// only structs carry per-field query semantics; a pointer to a
	// slice/map/primitive has nothing to walk
	if v.Kind() != reflect.Struct {
		return nil
	}

	// collect the query keys this struct accepts, then check each
	// incoming key against it
	known := make(map[string]bool)
	collectKnownFieldNames(v.Type(), known)
	// a bound type may accept filter keys its handler reads directly
	// rather than binding onto a field; fold those in so they pass the
	// unknown-key gate. assert on i, the original pointer, so a
	// value-receiver `ExtraQueryKeys()` stays in the method set.
	if ext, ok := i.(QueryKeyExtender); ok {
		for _, k := range ext.ExtraQueryKeys() {
			known[strings.ToLower(k)] = true
		}
	}
	if unknown := unknownQueryKeys(qp, known); len(unknown) > 0 {
		return fmt.Errorf("unknown query parameter(s): %s", strings.Join(unknown, ", "))
	}
	return bindStructFields(qp, v)
}

// collectKnownFieldNames walks structType and populates known with the
// lowercased Go field name for every exported field, recursing into
// anonymous embeds so their fields participate as if declared on the
// outer type.
func collectKnownFieldNames(structType reflect.Type, known map[string]bool) {
	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		if field.Anonymous && field.Type.Kind() == reflect.Struct {
			collectKnownFieldNames(field.Type, known)
			continue
		}
		if !field.IsExported() {
			continue
		}
		known[strings.ToLower(field.Name)] = true
	}
}

// unknownQueryKeys returns the sorted list of query keys that neither
// match a known struct field nor a reserved pagination param. The
// comparison lowercases each incoming key so a client varying key case
// is treated the same as the canonical lowercased form.
func unknownQueryKeys(qp url.Values, known map[string]bool) []string {
	var unknown []string
	for k := range qp {
		lower := strings.ToLower(k)
		if known[lower] || reservedQueryParams[lower] {
			continue
		}
		unknown = append(unknown, k)
	}
	sort.Strings(unknown)
	return unknown
}

// bindStructFields assigns each settable field of structValue from the
// query param matching its lowercased name. Recurses into anonymous
// embeds (Common, Definition, Instance, Reconciliation) so their
// fields participate as if declared on the outer type.
func bindStructFields(qp url.Values, structValue reflect.Value) error {
	structType := structValue.Type()
	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		fieldValue := structValue.Field(i)

		// anonymous embed: descend so its fields look like the outer's
		if field.Anonymous && fieldValue.Kind() == reflect.Struct {
			if err := bindStructFields(qp, fieldValue); err != nil {
				return err
			}
			continue
		}

		// skip unexported / un-settable fields (reflect won't let us
		// write to them anyway, and they aren't part of the API surface)
		if !fieldValue.CanSet() {
			continue
		}

		// URL param key is the lowercased Go field name; codegen rejects
		// any explicit query tag so there's no override to consult
		paramName := strings.ToLower(field.Name)

		// missing param means leave the field at its incoming value
		// (do not zero a pre-populated default)
		raw, ok := qp[paramName]
		if !ok || len(raw) == 0 {
			continue
		}

		// matched: convert the raw string into the field's actual type
		if err := writeFieldFromString(fieldValue, raw[0]); err != nil {
			return fmt.Errorf("QueryBinder: failed to bind %s=%q: %w", paramName, raw[0], err)
		}
	}
	return nil
}

// writeFieldFromString parses raw and writes the result through
// fieldValue into the underlying struct field. fieldValue must be
// settable; reflect.Value passes by value but holds a pointer to the
// original memory, so the SetString/SetInt/... calls below mutate the
// caller's struct. Pointer fields are allocated before recursion so
// callers don't have to pre-initialize. Unsupported kinds return an
// error rather than skip silently, so type drift surfaces at the first
// request.
func writeFieldFromString(fieldValue reflect.Value, raw string) error {
	// pointer field: allocate a new T, recurse to set its element, then
	// point fieldValue at it. Recursion is exactly one level. The inner
	// type is always a scalar by the time we land in the switch below.
	if fieldValue.Kind() == reflect.Ptr {
		elemValue := reflect.New(fieldValue.Type().Elem())
		if err := writeFieldFromString(elemValue.Elem(), raw); err != nil {
			return err
		}
		fieldValue.Set(elemValue)
		return nil
	}

	// scalar field: parse + set based on the field's kind
	switch fieldValue.Kind() {
	case reflect.String:
		fieldValue.SetString(raw)
	case reflect.Bool:
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return err
		}
		fieldValue.SetBool(parsed)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		parsed, err := strconv.ParseInt(raw, 10, fieldValue.Type().Bits())
		if err != nil {
			return err
		}
		fieldValue.SetInt(parsed)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		parsed, err := strconv.ParseUint(raw, 10, fieldValue.Type().Bits())
		if err != nil {
			return err
		}
		fieldValue.SetUint(parsed)
	case reflect.Float32, reflect.Float64:
		parsed, err := strconv.ParseFloat(raw, fieldValue.Type().Bits())
		if err != nil {
			return err
		}
		fieldValue.SetFloat(parsed)
	default:
		return fmt.Errorf("unsupported field kind: %s", fieldValue.Kind())
	}
	return nil
}
