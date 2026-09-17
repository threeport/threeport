package v0

import (
	"testing"

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
		ModRouter.routes.Range(func(key, _ any) bool {
			ModRouter.routes.Delete(key)

			return true
		})
	})

	return db
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
// by more than one row. beforeCreate rejects a duplicate, but Path carries no
// unique index, so the choice has to be defined rather than left to whichever
// row the database happens to return first.
func TestReconcileModuleRoute_PicksTheLowestIdForADuplicatePath(t *testing.T) {
	db := setupModuleRouterTestDB(t)
	firstApiId := seedModuleApi(t, db, "first", false)
	secondApiId := seedModuleApi(t, db, "second", true)

	// beforeCreate rejects a duplicate path, so the hooks are skipped to write
	// the state this is about: a row that got past the check-then-insert race,
	// or predates the check. Reconciliation has to be defined for it either way.
	unhooked := db.Session(&gorm.Session{SkipHooks: true})

	path := "/example.com/v0/widgets"
	first := ModuleApiRoute{Path: util.Ptr(path), ModuleApiID: &firstApiId}
	require.NoError(t, unhooked.Create(&first).Error)
	second := ModuleApiRoute{Path: util.Ptr(path), ModuleApiID: &secondApiId}
	require.NoError(t, unhooked.Create(&second).Error)
	require.Less(t, *first.ID, *second.ID)

	// the lower ID belongs to the non-core API, so the path is served; picking
	// the other row would have skipped it as core
	require.NoError(t, ReconcileModuleRoute(db, path))
	assert.True(t, routeServed(path), "the lowest ID wins, and that row's API is not core")
}
