package handlers

import (
	"fmt"
	"reflect"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// Handler is the main handler for the API server.  It contains the database connection,
// NATS connection, JetStream context, logger, and pagination mode.
type Handler struct {
	DB     *gorm.DB
	NC     *nats.Conn
	JS     nats.JetStreamContext
	Logger *zap.Logger

	// PaginationMode selects the strategy paginated list reads use. The
	// zero value means `PaginationModeAsOfSystemTime`, so a handler built
	// without one pages the way the flag's default does.
	PaginationMode apiserver_lib.PaginationMode
}

// New returns a new Handler. It leaves PaginationMode at its zero value, which
// reads as the default strategy; assign the field to select the other one.
func New(db *gorm.DB, nc *nats.Conn, rc nats.JetStreamContext, logger *zap.Logger) Handler {
	return Handler{DB: db, NC: nc, JS: rc, Logger: logger}
}

// RequestDB returns the handler DB scoped to the HTTP request, with query
// scopes applied and the request context attached so GORM honors client
// cancellation and hooks can read per-request state.
func (h Handler) RequestDB(c echo.Context) *gorm.DB {
	return h.DB.
		WithContext(c.Request().Context()).
		Scopes(apiserver_lib.QueryScopes(c)...)
}

// RespondBlockedDelete writes a 409 with blockers rendered as
// <api-namespace>/<kebab-kind>/<name>, falling back to id when no name resolves.
func RespondBlockedDelete(c echo.Context, db *gorm.DB, blocked *api_v0.BlockedDeleteError) error {
	baseType := *blocked.AttachedRefs[0].ObjectType
	baseID := *blocked.AttachedRefs[0].ObjectID
	idsByType := map[string]map[uint]struct{}{
		baseType: {baseID: struct{}{}},
	}
	for _, ref := range blocked.AttachedRefs {
		if idsByType[*ref.AttachedObjectType] == nil {
			idsByType[*ref.AttachedObjectType] = map[uint]struct{}{}
		}
		idsByType[*ref.AttachedObjectType][*ref.AttachedObjectID] = struct{}{}
	}

	// resolve names per object type; on lookup failure, fall back to an
	// empty map so one bad type doesn't drop every blocker from the response.
	namesByType := make(map[string]map[uint]string, len(idsByType))
	for objectType, idSet := range idsByType {
		ids := make([]uint, 0, len(idSet))
		for id := range idSet {
			ids = append(ids, id)
		}
		names, err := GetObjectNames(db, objectType, ids, false)
		if err != nil {
			names = map[uint]string{}
		}
		namesByType[objectType] = names
	}

	msg := api_v0.FormatBlockedDelete(blocked, namesByType)
	return apiserver_lib.ResponseStatus409(c, nil, fmt.Errorf("%s", msg), baseType)
}

// CreateMaterializedView creates a materialized view for a given object type and returns the
// view name and query ID. The views are used for result pagination in handlers that support pagination.
func (h Handler) CreateMaterializedView(queryTable string) (string, string, error) {
	// create the materialized view name
	viewName, queryId := GenerateMaterializedViewName()

	// create the materialized view
	createView := fmt.Sprintf("CREATE MATERIALIZED VIEW %s AS SELECT * FROM %s ORDER BY ID ASC", viewName, queryTable)
	if result := h.DB.Exec(createView); result.Error != nil {
		return "", "", fmt.Errorf("handler error: error creating materialized view with name %s: %w", viewName, result.Error)
	}

	// create an ID index on the materialized view
	createIdIndex := fmt.Sprintf("CREATE INDEX ON %s (ID)", viewName)
	if result := h.DB.Exec(createIdIndex); result.Error != nil {
		return "", "", fmt.Errorf("handler error: error creating ID index: %w", result.Error)
	}

	return viewName, queryId, nil
}

// GetMaterializedViewName finds the name of the materialized view created for a given pagination query ID.
// The query ID comes from the client, so it is checked against the shape the server issues and then
// matched as a bound parameter, keeping it out of the SQL text.
func (h Handler) GetMaterializedViewName(queryId string) (string, error) {
	if !apiserver_lib.ValidPaginationQueryId(queryId) {
		return "", fmt.Errorf("%w: not a server-issued pagination query id", apiserver_lib.ErrInvalidPaginationQueryId)
	}

	// find the materialized view name by query ID
	viewQuery := "SELECT table_name FROM information_schema.tables WHERE table_type = 'VIEW' AND table_name LIKE ?"
	viewPattern := fmt.Sprintf("%s_%%_%s", apiserver_lib.PaginationViewPrefix, queryId)
	var viewName string
	if result := h.DB.Raw(viewQuery, viewPattern).Scan(&viewName); result.Error != nil {
		return "", fmt.Errorf("handler error: error finding materialized view: %w", result.Error)
	}

	return viewName, nil
}

// GetMaterializedViewRecords fetches a page of records from a materialized
// view. query carries the caller's bound filter and the request's soft-delete
// scoping, and reading them off the view rather than baking them into its
// definition keeps every filter value a bound parameter. The view holds every
// column of the source table, so both predicates resolve against it. A cursor
// in pageParams resumes from the row after it.
func (h Handler) GetMaterializedViewRecords(
	query *gorm.DB,
	records interface{},
	viewName string,
	pageParams *apiserver_lib.PageRequestParams,
) (int64, error) {
	recordsQuery := query.Table(viewName)
	if pageParams.Cursor != 0 {
		recordsQuery = recordsQuery.Where("id > ?", pageParams.Cursor)
	}
	if result := recordsQuery.
		Order("id asc").
		Limit(int(pageParams.Limit)).
		Find(records); result.Error != nil {
		return 0, fmt.Errorf("handler error: error finding records: %w", result.Error)
	}

	return sliceLen(records)
}

// sliceLen returns the number of rows a Find wrote into records, which must be
// a pointer to a slice. Reflection is what lets the pagination helpers work
// with any api object's slice type.
func sliceLen(records interface{}) (int64, error) {
	recordsValue := reflect.ValueOf(records)
	if recordsValue.Kind() == reflect.Ptr {
		recordsValue = recordsValue.Elem()
	}
	if recordsValue.Kind() != reflect.Slice {
		return 0, fmt.Errorf("records must be a slice")
	}

	return int64(recordsValue.Len()), nil
}

// GenerateMaterializedViewName generates a standardized, unique materialized view name.
// The name contains the current timestamp for that is used to clean up materialized views
// that have passed a TTL. The query ID is used by the client to identify the view for
// subsequent pages of results.
func GenerateMaterializedViewName() (string, string) {
	queryId := util.RandomAlphaNumericString(16)
	viewName := fmt.Sprintf(
		"%s_%s_%s",
		apiserver_lib.PaginationViewPrefix,
		time.Now().Format("20060102150405"),
		queryId,
	)

	return viewName, queryId
}

// resolveHLCSnapshot returns the HLC timestamp a page of results reads at. An
// empty queryId anchors a new result set by capturing a fresh HLC, which the
// caller echoes back to the client so later pages land on the same snapshot. A
// non-empty one came from a client and is accepted only when it is a bare
// decimal, because it reaches the AS OF SYSTEM TIME clause as text rather than
// as a bind parameter.
func (h Handler) resolveHLCSnapshot(queryId string) (string, error) {
	if queryId == "" {
		var hlc string
		if result := h.DB.Raw("SELECT cluster_logical_timestamp()").Scan(&hlc); result.Error != nil {
			return "", fmt.Errorf("handler error: error capturing HLC snapshot: %w", result.Error)
		}
		// a scan that matched no row leaves the destination empty and
		// reports no error, and an empty token would reach the clause as
		// AS OF SYSTEM TIME '' and be echoed to the client as a queryid
		// that fails validation on every continuation
		if !apiserver_lib.ValidHLCToken(hlc) {
			return "", fmt.Errorf("handler error: error capturing HLC snapshot: got %q", hlc)
		}
		return hlc, nil
	}

	if !apiserver_lib.ValidHLCToken(queryId) {
		return "", fmt.Errorf("%w: not a valid HLC token", apiserver_lib.ErrInvalidPaginationQueryId)
	}

	return queryId, nil
}

// GetPaginatedRecordsAsOfSystemTime fetches a page of records from queryTable
// against a stable HLC snapshot without creating a materialized view. query
// carries the caller's bound filter and the request's soft-delete scoping, so
// a page holds the same rows the caller's count did. An empty queryId on
// pageParams captures a fresh HLC via `cluster_logical_timestamp()` and returns
// it so the caller can echo it back as the pagination queryId; a non-empty one
// is validated as an HLC token and used as the snapshot. The read runs AS OF
// SYSTEM TIME so every page sees the same snapshot under concurrent writes.
func (h Handler) GetPaginatedRecordsAsOfSystemTime(
	query *gorm.DB,
	records interface{},
	queryTable string,
	pageParams *apiserver_lib.PageRequestParams,
) (string, int64, error) {
	hlc, err := h.resolveHLCSnapshot(pageParams.QueryId)
	if err != nil {
		return "", 0, err
	}

	// the snapshot rides in the FROM clause as a raw table expression. It
	// is the one piece that cannot be a bound parameter, which is why the
	// token is validated above. Everything else, the caller's filter, the
	// soft-delete scoping, and the cursor, stays parameterized.
	recordsQuery := query.Clauses(clause.From{
		Tables: []clause.Table{{
			Name: fmt.Sprintf("%s AS OF SYSTEM TIME '%s'", queryTable, hlc),
			Raw:  true,
		}},
	})
	if pageParams.Cursor != 0 {
		recordsQuery = recordsQuery.Where("id > ?", pageParams.Cursor)
	}
	if result := recordsQuery.
		Order("id asc").
		Limit(int(pageParams.Limit)).
		Find(records); result.Error != nil {
		return hlc, 0, fmt.Errorf("handler error: error finding records: %w", result.Error)
	}

	count, err := sliceLen(records)
	if err != nil {
		return hlc, 0, err
	}

	return hlc, count, nil
}

// paginationMode returns the strategy this handler pages with. An unset field
// reads as the default, so a handler assembled without one still pages, and
// every paginating handler agrees on which strategy that is.
func (h Handler) paginationMode() apiserver_lib.PaginationMode {
	if h.PaginationMode == "" {
		return apiserver_lib.PaginationModeAsOfSystemTime
	}

	return h.PaginationMode
}

// DispatchGetPaginatedRecords fetches one page of results from queryTable
// using the handler's configured pagination strategy. query is the caller's
// request-scoped, model-bound, filtered db, and both strategies read through it
// so a page carries the same predicates the caller's count did. It returns the
// queryId to echo back to the client (a view-name suffix under
// `PaginationModeMaterializedView`, an HLC token under
// `PaginationModeAsOfSystemTime`), the row count for this page, and any error.
//
// Filters apply per request, not per snapshot, so a continuation has to repeat
// the same filter query params it sent on the first page. The generated
// handlers do this by binding the filter before every call.
//
// Under `PaginationModeMaterializedView` a view is created on the initial call
// (empty pageParams.QueryId) and looked up by queryId on continuation; the view
// is dropped once a page comes back shorter than the limit, so the TTL sweeper
// doesn't have to. A queryid naming no live view returns
// `ErrPaginationSessionExpired`, and so does a view that the sweeper drops
// between that lookup and the page read. Under `PaginationModeAsOfSystemTime` a
// fresh HLC is captured on the initial call and the caller's queryId is
// validated and reused on continuation; page-SELECT errors pass through
// `TranslatePaginationSessionError()` so a GC-expired snapshot reaches the
// client as the same expired-session error.
func (h Handler) DispatchGetPaginatedRecords(
	query *gorm.DB,
	records interface{},
	queryTable string,
	pageParams *apiserver_lib.PageRequestParams,
) (string, int64, error) {
	// take a session so every chained call below clones the statement
	// instead of mutating the caller's db, which leaves the caller free to
	// reuse it once the page has been fetched
	query = query.Session(&gorm.Session{})

	switch h.paginationMode() {
	case apiserver_lib.PaginationModeAsOfSystemTime:
		hlc, count, err := h.GetPaginatedRecordsAsOfSystemTime(query, records, queryTable, pageParams)
		if err != nil {
			return hlc, count, apiserver_lib.TranslatePaginationSessionError(err)
		}
		return hlc, count, nil

	case apiserver_lib.PaginationModeMaterializedView:
		var viewName string
		var queryId string
		if pageParams.QueryId == "" {
			// first page: build the view and use its suffix as queryId
			var err error
			viewName, queryId, err = h.CreateMaterializedView(queryTable)
			if err != nil {
				return "", 0, err
			}
		} else {
			// continuation: resolve the view the previous call created
			queryId = pageParams.QueryId
			var err error
			viewName, err = h.GetMaterializedViewName(pageParams.QueryId)
			if err != nil {
				return queryId, 0, err
			}
			// a queryid that names no live view means the snapshot is
			// gone, either dropped with the tail page below or swept by
			// the TTL. An empty name would otherwise build SQL with no
			// table and fail as a syntax error.
			if viewName == "" {
				return queryId, 0, apiserver_lib.ErrPaginationSessionExpired
			}
		}

		count, err := h.GetMaterializedViewRecords(query, records, viewName, pageParams)
		if err != nil {
			// a continuation races the TTL sweeper: the view resolved
			// above can be dropped before this read runs, and the
			// undefined-table failure that comes back means the snapshot
			// is gone rather than the server being broken. A first page
			// reads a view it created a moment earlier, so the same
			// failure there is a real fault and stays one.
			if pageParams.QueryId != "" {
				return queryId, count, apiserver_lib.TranslateDroppedViewError(err, viewName)
			}
			return queryId, count, err
		}

		// a page smaller than the limit is the tail: drop the view now
		// so the TTL cleanup doesn't have to catch it later. Failures
		// are logged and swallowed since the TTL sweep is the safety net.
		if count < pageParams.Limit {
			dropQuery := fmt.Sprintf("DROP MATERIALIZED VIEW IF EXISTS %s", viewName)
			if result := h.DB.Exec(dropQuery); result.Error != nil {
				h.Logger.Warn("handler error: error dropping materialized view", zap.String("viewName", viewName), zap.Error(result.Error))
			}
		}

		return queryId, count, nil

	default:
		return "", 0, fmt.Errorf("handler error: unknown pagination mode: %s", h.PaginationMode)
	}
}
