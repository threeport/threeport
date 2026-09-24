package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// newDryRunHandler returns a Handler whose DB builds SQL but never executes it,
// plus a pointer that receives the SQL of the last query built. The postgres
// dialector does not dial on open, so these tests need no CockroachDB and no
// control plane.
func newDryRunHandler(t *testing.T, mode apiserver_lib.PaginationMode) (Handler, *string) {
	t.Helper()

	db, err := gorm.Open(
		postgres.New(postgres.Config{
			DSN:                  "postgres://threeport:threeport@127.0.0.1:26257/threeport_api?sslmode=disable",
			PreferSimpleProtocol: true,
		}),
		&gorm.Config{DryRun: true, DisableAutomaticPing: true},
	)
	require.NoError(t, err, "opening a dry-run gorm DB must not dial")

	var captured string
	require.NoError(t, db.Callback().Query().After("gorm:query").Register(
		"test:capture_sql",
		func(tx *gorm.DB) { captured = tx.Statement.SQL.String() },
	))

	return Handler{DB: db, PaginationMode: mode, Logger: zap.NewNop()}, &captured
}

// newListContext returns the echo context a generated list handler receives for
// a GET at target, so QueryScopes sees the same query string production does.
func newListContext(target string) echo.Context {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	return e.NewContext(req, httptest.NewRecorder())
}

// listQuery builds the scoped, model-bound, filtered db the generated handlers
// hand to DispatchGetPaginatedRecords.
func (h Handler) listQuery(c echo.Context, filter *api_v0.KubernetesWorkloadDefinition) *gorm.DB {
	return h.RequestDB(c).Model(&api_v0.KubernetesWorkloadDefinition{}).Where(filter)
}

const testHLC = "1712345678901234567.0000000001"

// TestPaginatedReadCarriesFilterAndSoftDelete is the regression test for the
// hole this change closes. Before it, both pagination modes built
// "SELECT * FROM <table>" by hand, so a filtered list silently returned
// unfiltered rows and soft-deleted rows once the result set passed the page
// limit, even though the count that decided to paginate applied both.
func TestPaginatedReadCarriesFilterAndSoftDelete(t *testing.T) {
	name := "my-app"

	t.Run("as-of-system-time mode", func(t *testing.T) {
		h, sql := newDryRunHandler(t, apiserver_lib.PaginationModeAsOfSystemTime)
		c := newListContext("/v0/kubernetes-workload-definitions?name=" + name)
		records := &[]api_v0.KubernetesWorkloadDefinition{}

		_, _, err := h.DispatchGetPaginatedRecords(
			h.listQuery(c, &api_v0.KubernetesWorkloadDefinition{
				Definition: api_v0.Definition{Name: util.Ptr(name)},
			}),
			records,
			"v0_kubernetes_workload_definitions",
			&apiserver_lib.PageRequestParams{QueryId: testHLC, Cursor: 42, Limit: 100},
		)
		require.NoError(t, err)
		t.Logf("page SQL: %s", *sql)

		assert.Contains(t, *sql, "AS OF SYSTEM TIME '"+testHLC+"'", "the snapshot rides in the FROM clause")
		assert.Contains(t, *sql, "v0_kubernetes_workload_definitions", "the base table is still the source")
		assert.Contains(t, *sql, "name", "the bound filter reaches the page query")
		assert.Contains(t, *sql, "deleted_at", "soft-deleted rows stay out of the page")
		assert.Contains(t, *sql, "id > ", "the cursor advances the page")
		assert.Contains(t, strings.ToUpper(*sql), "LIMIT", "the page is bounded")
	})

	t.Run("materialized-view mode", func(t *testing.T) {
		h, sql := newDryRunHandler(t, apiserver_lib.PaginationModeMaterializedView)
		c := newListContext("/v0/kubernetes-workload-definitions?name=" + name)
		records := &[]api_v0.KubernetesWorkloadDefinition{}

		count, err := h.GetMaterializedViewRecords(
			h.listQuery(c, &api_v0.KubernetesWorkloadDefinition{
				Definition: api_v0.Definition{Name: util.Ptr(name)},
			}),
			records,
			"paginated_20260812000000_abc123",
			&apiserver_lib.PageRequestParams{Cursor: 42, Limit: 100},
		)
		require.NoError(t, err)
		assert.Equal(t, int64(0), count, "dry run writes no rows")

		assert.Contains(t, *sql, "paginated_20260812000000_abc123", "the view is the source")
		assert.Contains(t, *sql, "name", "the bound filter is applied when reading the view")
		assert.Contains(t, *sql, "deleted_at", "soft-deleted rows stay out of the page")
		assert.Contains(t, *sql, "id > ", "the cursor advances the page")
	})
}

// TestPaginatedReadHonorsIncludeDeleted verifies the soft-delete predicate is
// request-driven rather than hardcoded. A client asking for deleted rows with
// ?includedeleted=true has to get them on a paginated page too, which is why
// the fix carries the scoped db down instead of appending a fixed
// "deleted_at IS NULL" string.
func TestPaginatedReadHonorsIncludeDeleted(t *testing.T) {
	h, sql := newDryRunHandler(t, apiserver_lib.PaginationModeAsOfSystemTime)
	c := newListContext("/v0/kubernetes-workload-definitions?includedeleted=true")
	records := &[]api_v0.KubernetesWorkloadDefinition{}

	_, _, err := h.DispatchGetPaginatedRecords(
		h.listQuery(c, &api_v0.KubernetesWorkloadDefinition{}),
		records,
		"v0_kubernetes_workload_definitions",
		&apiserver_lib.PageRequestParams{QueryId: testHLC, Limit: 100},
	)
	require.NoError(t, err)

	assert.NotContains(t, *sql, "deleted_at", "includedeleted=true unscopes the paginated read")
	assert.Contains(t, *sql, "AS OF SYSTEM TIME", "the snapshot still applies")
}

// TestDispatchLeavesCallerQueryReusable verifies the dispatcher does not mutate
// the db it is handed. Materialized-view mode builds two statements from it, and
// a caller may reuse it afterwards; without the session clone the second build
// would inherit the first statement's conditions.
func TestDispatchLeavesCallerQueryReusable(t *testing.T) {
	h, sql := newDryRunHandler(t, apiserver_lib.PaginationModeAsOfSystemTime)
	c := newListContext("/v0/kubernetes-workload-definitions")
	query := h.listQuery(c, &api_v0.KubernetesWorkloadDefinition{})

	_, _, err := h.DispatchGetPaginatedRecords(
		query,
		&[]api_v0.KubernetesWorkloadDefinition{},
		"v0_kubernetes_workload_definitions",
		&apiserver_lib.PageRequestParams{QueryId: testHLC, Cursor: 42, Limit: 100},
	)
	require.NoError(t, err)
	require.Contains(t, *sql, "id > ")

	// reuse the same db the dispatcher was given; it must build a clean
	// statement rather than inherit the cursor predicate above
	reused := &[]api_v0.KubernetesWorkloadDefinition{}
	require.NoError(t, query.Find(reused).Error)

	assert.NotContains(t, *sql, "id > ", "the caller's db was mutated by the dispatcher")
	assert.NotContains(t, *sql, "AS OF SYSTEM TIME", "the caller's db kept the snapshot clause")
}

// TestDispatchRejectsInvalidHLCToken verifies a caller-supplied queryid that is
// not a bare decimal never reaches the AS OF SYSTEM TIME clause, which is the
// one place in the query that cannot be a bound parameter.
func TestDispatchRejectsInvalidHLCToken(t *testing.T) {
	h, _ := newDryRunHandler(t, apiserver_lib.PaginationModeAsOfSystemTime)
	c := newListContext("/v0/kubernetes-workload-definitions")

	_, _, err := h.DispatchGetPaginatedRecords(
		h.listQuery(c, &api_v0.KubernetesWorkloadDefinition{}),
		&[]api_v0.KubernetesWorkloadDefinition{},
		"v0_kubernetes_workload_definitions",
		&apiserver_lib.PageRequestParams{QueryId: "1.0'; DROP TABLE v0_events; --", Cursor: 1, Limit: 100},
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a valid HLC token")
	assert.ErrorIs(t, err, apiserver_lib.ErrInvalidPaginationQueryId,
		"the generated handlers match on this to answer 400 rather than 500")
}

// TestViewLookupRejectsInjectedQueryId verifies a caller-supplied queryid that
// is not one the server issued never reaches the materialized-view lookup. The
// lookup matches the queryid inside a LIKE pattern, so an unchecked value that
// closed the string literal could read any table name out of
// information_schema and steer the page query that follows.
func TestViewLookupRejectsInjectedQueryId(t *testing.T) {
	injected := "' UNION SELECT table_name FROM information_schema.tables WHERE '1'='1"

	t.Run("through the dispatcher", func(t *testing.T) {
		h, _ := newDryRunHandler(t, apiserver_lib.PaginationModeMaterializedView)
		c := newListContext("/v0/kubernetes-workload-definitions")

		_, _, err := h.DispatchGetPaginatedRecords(
			h.listQuery(c, &api_v0.KubernetesWorkloadDefinition{}),
			&[]api_v0.KubernetesWorkloadDefinition{},
			"v0_kubernetes_workload_definitions",
			&apiserver_lib.PageRequestParams{QueryId: injected, Cursor: 1, Limit: 100},
		)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a server-issued pagination query id")
		assert.ErrorIs(t, err, apiserver_lib.ErrInvalidPaginationQueryId,
			"the generated handlers match on this to answer 400 rather than 500")
	})

	t.Run("at the lookup itself", func(t *testing.T) {
		h, _ := newDryRunHandler(t, apiserver_lib.PaginationModeMaterializedView)

		viewName, err := h.GetMaterializedViewName(injected)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a server-issued pagination query id")
		assert.ErrorIs(t, err, apiserver_lib.ErrInvalidPaginationQueryId,
			"the generated handlers match on this to answer 400 rather than 500")
		assert.Empty(t, viewName, "no view name is returned for a rejected queryid")
	})
}

// TestUnsetModePagesAsOfSystemTime verifies both paginating call sites read an
// unset mode the same way. The api-server assigns the mode from a validated
// flag, so an empty field means a handler assembled without one, and the two
// used to disagree: the dispatcher paged against an HLC snapshot while the
// events handler fell through to a materialized view.
func TestUnsetModePagesAsOfSystemTime(t *testing.T) {
	t.Run("through the dispatcher", func(t *testing.T) {
		h, sql := newDryRunHandler(t, "")
		c := newListContext("/v0/kubernetes-workload-definitions")

		_, _, err := h.DispatchGetPaginatedRecords(
			h.listQuery(c, &api_v0.KubernetesWorkloadDefinition{}),
			&[]api_v0.KubernetesWorkloadDefinition{},
			"v0_kubernetes_workload_definitions",
			&apiserver_lib.PageRequestParams{QueryId: testHLC, Cursor: 42, Limit: 100},
		)

		require.NoError(t, err)
		assert.Contains(t, *sql, "AS OF SYSTEM TIME '"+testHLC+"'")
	})

	t.Run("through the events handler", func(t *testing.T) {
		h, sql := newDryRunHandler(t, "")
		c, rec := newHandlerRequest(clientErrorCase{
			method: http.MethodGet,
			route:  "/v0/events",
			target: "/v0/events?queryid=" + testHLC + "&cursor=42&limit=100",
		})

		require.NoError(t, h.GetEventsJoinAttachedObjectReferences(c))

		// the materialized-view branch reads the queryid as a view suffix
		// and rejects an HLC token as malformed, so the client's own
		// snapshot came back as a client error before this fix
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, *sql, "AS OF SYSTEM TIME '"+testHLC+"'")
	})
}

// TestDispatchRejectsUnknownMode verifies a mode that reached the handler
// without passing ValidPaginationMode is refused rather than silently paging
// through one of the two real strategies.
func TestDispatchRejectsUnknownMode(t *testing.T) {
	h, _ := newDryRunHandler(t, apiserver_lib.PaginationMode("snapshot"))
	c := newListContext("/v0/kubernetes-workload-definitions")

	_, _, err := h.DispatchGetPaginatedRecords(
		h.listQuery(c, &api_v0.KubernetesWorkloadDefinition{}),
		&[]api_v0.KubernetesWorkloadDefinition{},
		"v0_kubernetes_workload_definitions",
		&apiserver_lib.PageRequestParams{Limit: 100},
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown pagination mode")
}
