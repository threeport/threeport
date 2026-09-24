package v0

import (
	"strings"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// QueryParamIncludeDeleted is the URL query parameter that bypasses the
// soft-delete filter on Get handlers when set to "true". Used for historical
// lookups such as resolving the name of an object that has since been deleted.
const QueryParamIncludeDeleted = "includedeleted"

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
	return scopes
}
