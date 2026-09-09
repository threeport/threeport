package v0

// PaginationMode selects the strategy the API server uses to keep a
// pagination window's snapshot stable across pages.
type PaginationMode string

const (
	// PaginationModeAsOfSystemTime pages against an HLC-encoded snapshot
	// carried in QueryId and read back with AS OF SYSTEM TIME.
	PaginationModeAsOfSystemTime PaginationMode = "as-of-system-time"

	// PaginationModeMaterializedView pages against a per-query materialized
	// view named by QueryId.
	PaginationModeMaterializedView PaginationMode = "materialized-view"
)

// ValidPaginationMode reports whether v is one of the supported modes.
func ValidPaginationMode(v string) bool {
	switch PaginationMode(v) {
	case PaginationModeAsOfSystemTime, PaginationModeMaterializedView:
		return true
	}
	return false
}
