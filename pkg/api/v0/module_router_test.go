package v0

import (
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// setupModuleRouterTestDB returns an in-memory database with the two tables
// reconciliation reads, and leaves the process-wide router empty for the test.
func setupModuleRouterTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	// ModuleApiRoute.ModuleApiID carries a relationship tag, so creating one
	// writes an attached object reference in the same transaction
	require.NoError(t, db.AutoMigrate(&ModuleApi{}, &ModuleApiRoute{}, &AttachedObjectReference{}))

	t.Cleanup(func() {
		// InitModuleRouter and the resync tests leave a goroutine reading this
		// database; it has to be gone before the next test replaces either
		stopModuleRouteResync()
		ModRouter.routes.Range(func(key, _ any) bool {
			ModRouter.routes.Delete(key)

			return true
		})
		pendingModuleRoutes.Range(func(key, _ any) bool {
			pendingModuleRoutes.Delete(key)

			return true
		})
		// the repair goroutine exits once nothing is pending; wait so it does
		// not outlive the test and touch the next one's router
		for moduleRouteRepairRunning.Load() {
			time.Sleep(time.Millisecond)
		}
	})

	return db
}

// dropPathUniqueIndex models a control plane upgraded into the current model
// rather than created from it. The migration leaves an existing table alone, so
// the unique index the model declares on Path is not necessarily there, and
// duplicate rows for one path remain possible.
func dropPathUniqueIndex(t *testing.T, db *gorm.DB) {
	t.Helper()

	require.NoError(t, db.Migrator().DropIndex(&ModuleApiRoute{}, "Path"))
}

// routeServed reports whether the router currently holds an entry for a path.
func routeServed(path string) bool {
	_, ok := ModRouter.routes.Load(path)

	return ok
}

// seedModuleApi writes a module API and returns its ID.
func seedModuleApi(t *testing.T, db *gorm.DB, name string, core bool) uint {
	t.Helper()

	modApi := ModuleApi{
		Name:     util.Ptr(name),
		Core:     util.Ptr(core),
		Endpoint: util.Ptr("module-api.example.svc:443"),
	}
	require.NoError(t, db.Create(&modApi).Error)

	return *modApi.ID
}

// TestReconcileModuleRoute_AddsACommittedRoute covers the ordinary create: the
// row is there, so the router serves the path.
func TestReconcileModuleRoute_AddsACommittedRoute(t *testing.T) {
	db := setupModuleRouterTestDB(t)
	moduleApiId := seedModuleApi(t, db, "example", false)

	route := ModuleApiRoute{Path: util.Ptr("/example.com/v0/widgets"), ModuleApiID: &moduleApiId}
	require.NoError(t, db.Create(&route).Error)

	require.NoError(t, ReconcileModuleRoute(db, *route.Path))
	assert.True(t, routeServed(*route.Path))
}

// TestReconcileModuleRoute_DropsARouteWithNoRow is the failure the hooks caused.
// A create whose transaction did not commit leaves no row, and the router must
// not be left proxying to one that does not exist.
func TestReconcileModuleRoute_DropsARouteWithNoRow(t *testing.T) {
	db := setupModuleRouterTestDB(t)
	path := "/example.com/v0/widgets"

	// stand in for a route the old afterCreate hook registered from inside a
	// transaction that then rolled back
	ModRouter.AddRoute(path, nil)
	require.True(t, routeServed(path))

	require.NoError(t, ReconcileModuleRoute(db, path))
	assert.False(t, routeServed(path), "a path with no committed row must not be served")
}

// TestReconcileModuleRoute_KeepsARouteWhoseRowSurvived is the mirror failure. A
// delete that rolled back left the row in place, and removing the route from
// inside the transaction stopped a valid endpoint being served until restart.
func TestReconcileModuleRoute_KeepsARouteWhoseRowSurvived(t *testing.T) {
	db := setupModuleRouterTestDB(t)
	moduleApiId := seedModuleApi(t, db, "example", false)

	route := ModuleApiRoute{Path: util.Ptr("/example.com/v0/widgets"), ModuleApiID: &moduleApiId}
	require.NoError(t, db.Create(&route).Error)

	require.NoError(t, ReconcileModuleRoute(db, *route.Path))
	assert.True(t, routeServed(*route.Path), "the row committed, so the route has to be served")
}

// TestReconcileModuleRoute_SkipsCoreApis covers the check the old hook made:
// core routes are served by this process and must not be proxied.
func TestReconcileModuleRoute_SkipsCoreApis(t *testing.T) {
	db := setupModuleRouterTestDB(t)
	moduleApiId := seedModuleApi(t, db, "core", true)

	route := ModuleApiRoute{Path: util.Ptr("/v0/widgets"), ModuleApiID: &moduleApiId}
	require.NoError(t, db.Create(&route).Error)

	require.NoError(t, ReconcileModuleRoute(db, *route.Path))
	assert.False(t, routeServed(*route.Path), "a core API path is served directly, not proxied")
}

// TestReconcileModuleRoute_IsIdempotent covers repeated calls, which is what a
// retried request produces.
func TestReconcileModuleRoute_IsIdempotent(t *testing.T) {
	db := setupModuleRouterTestDB(t)
	moduleApiId := seedModuleApi(t, db, "example", false)

	route := ModuleApiRoute{Path: util.Ptr("/example.com/v0/widgets"), ModuleApiID: &moduleApiId}
	require.NoError(t, db.Create(&route).Error)

	for range 3 {
		require.NoError(t, ReconcileModuleRoute(db, *route.Path))
	}
	assert.True(t, routeServed(*route.Path))

	require.NoError(t, db.Unscoped().Delete(&route).Error)
	for range 3 {
		require.NoError(t, ReconcileModuleRoute(db, *route.Path))
	}
	assert.False(t, routeServed(*route.Path))
}

// TestReconcileModuleRoute_PicksTheLowestIdForADuplicatePath covers a path named
// by more than one row, which the unique index on Path rules out for a schema
// created from the current model but not for one upgraded into it.
//
// The rows are written in the opposite order to their IDs to state the intent,
// but this cannot fail against an unordered query: sqlite returns rows by
// rowid, which is the ID. CockroachDB does not promise an order without ORDER
// BY, which is why the query has one - that part is argued, not covered here.
func TestReconcileModuleRoute_PicksTheLowestIdForADuplicatePath(t *testing.T) {
	db := setupModuleRouterTestDB(t)
	dropPathUniqueIndex(t, db)
	coreApiId := seedModuleApi(t, db, "core", true)
	moduleApiId := seedModuleApi(t, db, "example", false)

	unhooked := db.Session(&gorm.Session{SkipHooks: true})
	path := "/example.com/v0/widgets"

	// written first, higher ID: the row an unordered query returns first
	proxied := ModuleApiRoute{
		Common:      Common{ID: util.Ptr(uint(2))},
		Path:        util.Ptr(path),
		ModuleApiID: &moduleApiId,
	}
	require.NoError(t, unhooked.Create(&proxied).Error)
	core := ModuleApiRoute{
		Common:      Common{ID: util.Ptr(uint(1))},
		Path:        util.Ptr(path),
		ModuleApiID: &coreApiId,
	}
	require.NoError(t, unhooked.Create(&core).Error)

	require.NoError(t, ReconcileModuleRoute(db, path))
	assert.False(
		t, routeServed(path),
		"the lowest ID wins, and that row's API is core, so the path is not proxied",
	)
}

// TestInitModuleRouter_AgreesWithReconciliation covers the two rules problem.
// Startup used to register every route belonging to a non-core API, which meant
// a duplicated path where the lowest ID is core was served after a restart and
// removed by reconciliation - the router's contents depending on which of the
// two last ran.
func TestInitModuleRouter_AgreesWithReconciliation(t *testing.T) {
	db := setupModuleRouterTestDB(t)
	dropPathUniqueIndex(t, db)
	coreApiId := seedModuleApi(t, db, "core", true)
	moduleApiId := seedModuleApi(t, db, "example", false)

	unhooked := db.Session(&gorm.Session{SkipHooks: true})

	// the core row takes the lower ID, so reconciliation rejects the path
	path := "/example.com/v0/widgets"
	core := ModuleApiRoute{Path: util.Ptr(path), ModuleApiID: &coreApiId}
	require.NoError(t, unhooked.Create(&core).Error)
	proxied := ModuleApiRoute{Path: util.Ptr(path), ModuleApiID: &moduleApiId}
	require.NoError(t, unhooked.Create(&proxied).Error)
	require.Less(t, *core.ID, *proxied.ID)

	// a path only the non-core API names, to show startup still registers
	plainPath := "/example.com/v0/gadgets"
	plain := ModuleApiRoute{Path: util.Ptr(plainPath), ModuleApiID: &moduleApiId}
	require.NoError(t, unhooked.Create(&plain).Error)

	require.NoError(t, InitModuleRouter(db, echo.New(), false))

	assert.False(
		t, routeServed(path),
		"startup has to reach the same answer reconciliation does for a duplicated path",
	)
	assert.True(t, routeServed(plainPath), "an unambiguous path is still served")

	// and reconciliation run afterwards must not change what startup produced
	require.NoError(t, ReconcileModuleRoute(db, path))
	require.NoError(t, ReconcileModuleRoute(db, plainPath))
	assert.False(t, routeServed(path))
	assert.True(t, routeServed(plainPath))
}

// failingModuleRouterDB returns a database and a switch that makes its reads
// fail, which is what a briefly unreachable control plane database looks like
// to reconciliation: one handle, not a different one.
func failingModuleRouterDB(t *testing.T) (*gorm.DB, *atomic.Bool) {
	t.Helper()

	db := setupModuleRouterTestDB(t)

	var failing atomic.Bool
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(
		"test:fail_reads",
		func(tx *gorm.DB) {
			if failing.Load() {
				tx.AddError(errors.New("database unavailable"))
			}
		},
	))

	return db, &failing
}

// withFastModuleRouteRepair shrinks the retry waits so a test does not sit
// through them.
func withFastModuleRouteRepair(t *testing.T) {
	t.Helper()

	attempts, seconds, wait := moduleRouteReconcileAttempts,
		moduleRouteReconcileWaitSeconds, moduleRouteRepairWait
	moduleRouteReconcileAttempts, moduleRouteReconcileWaitSeconds = 1, 0
	moduleRouteRepairWait = time.Millisecond

	t.Cleanup(func() {
		moduleRouteReconcileAttempts, moduleRouteReconcileWaitSeconds = attempts, seconds
		moduleRouteRepairWait = wait
	})
}

// TestReconcileModuleRoute_RepairsAPathAFailedReadLeftBehind covers the window
// the retry does not close. The write has already committed when reconciliation
// runs, and the request cannot be replayed to try again - a repeated create
// answers 409 and a repeated delete answers 404 - so a path whose read failed
// has to be picked up rather than wait for a restart.
func TestReconcileModuleRoute_RepairsAPathAFailedReadLeftBehind(t *testing.T) {
	withFastModuleRouteRepair(t)
	db, failing := failingModuleRouterDB(t)
	moduleApiId := seedModuleApi(t, db, "example", false)

	failedPath := "/example.com/v0/widgets"
	require.NoError(t, db.Create(&ModuleApiRoute{
		Path: util.Ptr(failedPath), ModuleApiID: &moduleApiId,
	}).Error)
	otherPath := "/example.com/v0/gadgets"
	require.NoError(t, db.Create(&ModuleApiRoute{
		Path: util.Ptr(otherPath), ModuleApiID: &moduleApiId,
	}).Error)

	// the route is committed but the database cannot be read, so the router is
	// not told and the handler has already answered 500
	failing.Store(true)
	require.Error(t, ReconcileModuleRoute(db, failedPath))
	require.False(t, routeServed(failedPath))

	// the next reconciliation of any path repairs it
	failing.Store(false)
	require.NoError(t, ReconcileModuleRoute(db, otherPath))

	assert.True(t, routeServed(failedPath), "the path a failed read left behind has to be repaired")
	assert.True(t, routeServed(otherPath))
}

// TestReconcileModuleRoute_RepairsWithoutAnotherRouteChange is the case waiting
// on the next reconciliation does not cover. Nothing guarantees another module
// route is ever created or deleted, so a path left behind by a failed read has
// to be retried on its own rather than stay wrong until the process restarts.
func TestReconcileModuleRoute_RepairsWithoutAnotherRouteChange(t *testing.T) {
	withFastModuleRouteRepair(t)
	db, failing := failingModuleRouterDB(t)
	moduleApiId := seedModuleApi(t, db, "example", false)

	path := "/example.com/v0/widgets"
	require.NoError(t, db.Create(&ModuleApiRoute{
		Path: util.Ptr(path), ModuleApiID: &moduleApiId,
	}).Error)

	failing.Store(true)
	require.Error(t, ReconcileModuleRoute(db, path))
	require.False(t, routeServed(path))

	// the database comes back and nothing else touches a module route
	failing.Store(false)

	assert.Eventually(
		t, func() bool { return routeServed(path) }, 5*time.Second, 5*time.Millisecond,
		"the route has to be repaired without waiting for another route change",
	)
	assert.Eventually(
		t, func() bool { return !moduleRouteRepairRunning.Load() }, time.Second, 5*time.Millisecond,
		"the repair goroutine has to stop once nothing is pending",
	)
}

// TestReconcileModuleRoutes_PicksUpAnotherProcessesCreate covers the half of
// the problem post-commit reconciliation cannot reach. The router is
// process-local and only the process that handled the write reconciles, so a
// route committed by one rest-api replica is not proxied by its siblings. Here
// the row is written without any reconciliation, standing in for the sibling
// that committed it.
func TestReconcileModuleRoutes_PicksUpAnotherProcessesCreate(t *testing.T) {
	db := setupModuleRouterTestDB(t)
	moduleApiId := seedModuleApi(t, db, "example", false)

	path := "/example.com/v0/widgets"
	require.NoError(t, db.Create(&ModuleApiRoute{
		Path: util.Ptr(path), ModuleApiID: &moduleApiId,
	}).Error)
	require.False(t, routeServed(path), "this process has not been told about it")

	require.NoError(t, ReconcileModuleRoutes(db))
	assert.True(t, routeServed(path))
}

// TestReconcileModuleRoutes_PicksUpAnotherProcessesDelete is the mirror. A row
// deleted elsewhere leaves this process proxying a path the table no longer
// has, and the database alone cannot say so - the path is gone from it, so
// walking only the committed rows would never visit it.
func TestReconcileModuleRoutes_PicksUpAnotherProcessesDelete(t *testing.T) {
	db := setupModuleRouterTestDB(t)
	moduleApiId := seedModuleApi(t, db, "example", false)

	path := "/example.com/v0/widgets"
	route := ModuleApiRoute{Path: util.Ptr(path), ModuleApiID: &moduleApiId}
	require.NoError(t, db.Create(&route).Error)
	require.NoError(t, ReconcileModuleRoute(db, path))
	require.True(t, routeServed(path))

	// the sibling's delete, which this process is not told about
	require.NoError(t, db.Unscoped().Delete(&route).Error)

	require.NoError(t, ReconcileModuleRoutes(db))
	assert.False(t, routeServed(path), "a path the table no longer has must stop being proxied")
}

// TestReconcileModuleRoutes_LeavesAgreeingRoutesAlone covers the ordinary pass
// on a single-replica install, where the process already agrees with the table.
func TestReconcileModuleRoutes_LeavesAgreeingRoutesAlone(t *testing.T) {
	db := setupModuleRouterTestDB(t)
	moduleApiId := seedModuleApi(t, db, "example", false)

	paths := []string{"/example.com/v0/widgets", "/example.com/v0/gadgets"}
	for _, path := range paths {
		require.NoError(t, db.Create(&ModuleApiRoute{
			Path: util.Ptr(path), ModuleApiID: &moduleApiId,
		}).Error)
		require.NoError(t, ReconcileModuleRoute(db, path))
	}

	require.NoError(t, ReconcileModuleRoutes(db))
	for _, path := range paths {
		assert.True(t, routeServed(path), path)
	}
}

// TestStartModuleRouteResync_ConvergesWithoutAnyLocalWrite drives the goroutine
// InitModuleRouter starts: nothing happens in this process, and the route a
// sibling committed still arrives.
func TestStartModuleRouteResync_ConvergesWithoutAnyLocalWrite(t *testing.T) {
	interval := moduleRouteResyncInterval
	moduleRouteResyncInterval = time.Millisecond
	t.Cleanup(func() { moduleRouteResyncInterval = interval })

	db := setupModuleRouterTestDB(t)
	moduleApiId := seedModuleApi(t, db, "example", false)

	startModuleRouteResync(db)

	path := "/example.com/v0/widgets"
	require.NoError(t, db.Create(&ModuleApiRoute{
		Path: util.Ptr(path), ModuleApiID: &moduleApiId,
	}).Error)

	assert.Eventually(
		t, func() bool { return routeServed(path) }, 5*time.Second, 5*time.Millisecond,
		"a route committed by another process has to arrive without one being written here",
	)
}

// TestStartModuleRouteResync_StartsOneGoroutine covers the guard, since
// InitModuleRouter can be called more than once in a process under test.
func TestStartModuleRouteResync_StartsOneGoroutine(t *testing.T) {
	db := setupModuleRouterTestDB(t)

	startModuleRouteResync(db)
	before := runtime.NumGoroutine()
	for range 5 {
		startModuleRouteResync(db)
	}

	assert.LessOrEqual(t, runtime.NumGoroutine(), before, "a second call must not start another")
}
