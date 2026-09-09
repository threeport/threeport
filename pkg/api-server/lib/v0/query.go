package v0

import (
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// QueryParamIncludeDeleted is the URL query parameter that bypasses the
// soft-delete filter on Get handlers when set to "true". Used for historical
// lookups such as resolving the name of an object that has since been deleted.
const QueryParamIncludeDeleted = "includedeleted"

// QueryParamIDs is the URL query parameter that restricts a list to the listed
// object ids, comma-separated. An empty list or tokens that are not uints is no filter.
const QueryParamIDs = "ids"

// LiveRowsFilter returns a SQL fragment that excludes soft-deleted
// rows for each given table alias, joined by AND. Compose it into
// raw-string queries built via .Table() or .Joins(), which bypass
// gorm's automatic deleted_at filter because there is no Go struct
// for gorm to read tags from. Pass the alias as it appears in the
// query, or the bare table name when no alias is given.
func LiveRowsFilter(aliases ...string) string {
	parts := make([]string, len(aliases))
	for i, alias := range aliases {
		parts[i] = alias + ".deleted_at IS NULL"
	}
	return strings.Join(parts, " AND ")
}

// QueryScopes returns the gorm scopes derived from URL query parameters on c.
// Each entry transforms a *gorm.DB to apply one request-driven behavior. New
// behavior modifiers are added here; generated handlers don't change.
func QueryScopes(c echo.Context) []func(*gorm.DB) *gorm.DB {
	var scopes []func(*gorm.DB) *gorm.DB
	if c.QueryParam(QueryParamIncludeDeleted) == "true" {
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Unscoped()
		})
	}
	if ids := parseIDsQueryParam(c.QueryParam(QueryParamIDs)); len(ids) > 0 {
		scopes = append(scopes, func(db *gorm.DB) *gorm.DB {
			return db.Where("id IN ?", ids)
		})
	}
	return scopes
}

// parseIDsQueryParam splits a comma-separated list of ids, skipping empty and
// unparseable tokens. No valid entries returns nil so the caller adds no scope.
func parseIDsQueryParam(raw string) []uint {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]uint, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		parsed, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			continue
		}
		out = append(out, uint(parsed))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
