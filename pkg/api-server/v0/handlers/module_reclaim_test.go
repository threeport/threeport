package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	sqlite "gorm.io/driver/sqlite"
	"gorm.io/gorm"

	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

const reclaimRoute = api_v0.PathModuleControllerReclaim

// newReclaimHandler returns a handler backed by an in-memory database holding
// one module controller bound to ownerID and, when ownerAlive is set, a module
// api with that ID. It returns the controller's ID.
func newReclaimHandler(t *testing.T, ownerID uint, ownerAlive bool) (Handler, *gorm.DB, uint) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	// one connection so every statement sees the same in-memory database
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(
		&api_v0.ModuleApi{}, &api_v0.ModuleApiRoute{}, &api_v0.ModuleObject{}, &api_v0.ModuleController{},
		&api_v0.AttachedObjectReference{},
	))

	if ownerAlive {
		require.NoError(t, db.Create(&api_v0.ModuleApi{
			Common:   api_v0.Common{ID: util.Ptr(ownerID)},
			Name:     util.Ptr("owner-module"),
			Endpoint: util.Ptr("owner:1234"),
		}).Error)
	}

	controller := api_v0.ModuleController{
		Name:           util.Ptr("widget-controller"),
		DeploymentName: util.Ptr("old-ns/old-deployment"),
		ModuleApiID:    util.Ptr(ownerID),
	}
	require.NoError(t, db.Create(&controller).Error)

	registerValidateTags(api_v0.ObjectTypeModuleController, new(api_v0.ModuleController))

	return Handler{DB: db, Logger: zap.NewNop()}, db, *controller.ID
}

// reclaimBody is the request a registering module sends for moduleApiID.
func reclaimBody(moduleApiID uint) string {
	body, _ := json.Marshal(map[string]any{
		"Name":           "widget-controller",
		"DeploymentName": "new-ns/new-deployment",
		"ModuleApiID":    moduleApiID,
	})
	return string(body)
}

// loadController reads the controller row back.
func loadController(t *testing.T, db *gorm.DB, id uint) api_v0.ModuleController {
	t.Helper()
	var controller api_v0.ModuleController
	require.NoError(t, db.First(&controller, id).Error)
	return controller
}

// requiredByModuleApiIDs lists the module apis a live `requires` reference names
// for the controller.
func requiredByModuleApiIDs(t *testing.T, db *gorm.DB, controllerID uint) []uint {
	t.Helper()
	var refs []api_v0.AttachedObjectReference
	require.NoError(t, db.Where(
		"object_type = ? AND attached_object_type = ? AND attached_object_id = ?",
		new(api_v0.ModuleApi).GetFullyQualifiedType(),
		new(api_v0.ModuleController).GetFullyQualifiedType(),
		controllerID,
	).Order("object_id").Find(&refs).Error)
	ids := []uint{}
	for _, ref := range refs {
		ids = append(ids, *ref.ObjectID)
	}
	return ids
}

func TestReclaimModuleControllerRebindsOrphanedRow(t *testing.T) {
	// the prior owner, module api 1, no longer exists
	h, db, controllerID := newReclaimHandler(t, 1, false)

	c, rec := newUniqueViolationRequest(reclaimRoute, reclaimBody(2))
	require.NoError(t, h.ReclaimModuleController(c))

	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	controller := loadController(t, db, controllerID)
	assert.Equal(t, uint(2), *controller.ModuleApiID)
	assert.Equal(t, "new-ns/new-deployment", *controller.DeploymentName)

	// the `requires` reference moves with the row: only the new owner is named
	assert.Equal(t, []uint{2}, requiredByModuleApiIDs(t, db, controllerID))
}

func TestReclaimModuleControllerRefusesActiveOwner(t *testing.T) {
	// module api 1 is alive and owns the controller
	h, db, controllerID := newReclaimHandler(t, 1, true)

	c, rec := newUniqueViolationRequest(reclaimRoute, reclaimBody(2))
	require.NoError(t, h.ReclaimModuleController(c))

	assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	controller := loadController(t, db, controllerID)
	assert.Equal(t, uint(1), *controller.ModuleApiID, "the active owner keeps its controller")
	assert.Equal(t, "old-ns/old-deployment", *controller.DeploymentName)
	assert.Equal(t, []uint{1}, requiredByModuleApiIDs(t, db, controllerID))
}

func TestReclaimModuleControllerIsIdempotentForCurrentOwner(t *testing.T) {
	// the registering module already holds the controller
	h, db, controllerID := newReclaimHandler(t, 1, true)

	c, rec := newUniqueViolationRequest(reclaimRoute, reclaimBody(1))
	require.NoError(t, h.ReclaimModuleController(c))

	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	controller := loadController(t, db, controllerID)
	assert.Equal(t, uint(1), *controller.ModuleApiID)
	assert.Equal(t, "old-ns/old-deployment", *controller.DeploymentName,
		"a controller the module already holds is returned unchanged")
}

func TestReclaimModuleControllerCreatesWhenRowIsGone(t *testing.T) {
	h, db, controllerID := newReclaimHandler(t, 1, false)
	// the conflicting row was deleted after the caller's create failed
	require.NoError(t, db.Delete(&api_v0.ModuleController{}, controllerID).Error)

	c, rec := newUniqueViolationRequest(reclaimRoute, reclaimBody(2))
	require.NoError(t, h.ReclaimModuleController(c))

	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var controllers []api_v0.ModuleController
	require.NoError(t, db.Find(&controllers).Error)
	require.Len(t, controllers, 1)
	assert.Equal(t, uint(2), *controllers[0].ModuleApiID)
}

func TestReclaimModuleControllerRejectsIncompletePayload(t *testing.T) {
	h, _, _ := newReclaimHandler(t, 1, false)

	c, rec := newUniqueViolationRequest(reclaimRoute, `{"Name":"widget-controller"}`)
	require.NoError(t, h.ReclaimModuleController(c))

	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

// TestReclaimModuleControllerDoesNotStealRowRebindDuringCheck is the
// regression test for threeport#575. The owner-liveness read finds the prior
// owner gone; before the write lands, the original module re-registers and
// rebinds the row to its new module api. The write must not overwrite that
// binding.
//
// sqlite cannot abort the transaction the way CockroachDB's serializable
// isolation does, so this exercises the guard that holds on any database:
// the rebind names the owner it checked, matches no rows when the owner has
// changed, and the request answers 409.
func TestReclaimModuleControllerDoesNotStealRowRebindDuringCheck(t *testing.T) {
	// module api 1 is gone, so the check passes
	h, db, controllerID := newReclaimHandler(t, 1, false)

	// right after the owner-liveness read, the original module comes back as
	// module api 7 and rebinds the row, on the same connection as the request
	moduleApiTable := (&api_v0.ModuleApi{}).TableName()
	raced := false
	require.NoError(t, db.Callback().Query().After("gorm:query").Register(
		"test:rebind_after_owner_check",
		func(tx *gorm.DB) {
			if raced || tx.Statement.Schema == nil || tx.Statement.Schema.Table != moduleApiTable {
				return
			}
			raced = true
			other := tx.Session(&gorm.Session{NewDB: true})
			require.NoError(t, other.Exec(
				"INSERT INTO "+moduleApiTable+" (id, name, endpoint) VALUES (7, 'owner-module', 'owner:1234')",
			).Error)
			require.NoError(t, other.Exec(
				"UPDATE "+(&api_v0.ModuleController{}).TableName()+" SET module_api_id = 7 WHERE id = ?",
				controllerID,
			).Error)
		},
	))

	c, rec := newUniqueViolationRequest(reclaimRoute, reclaimBody(2))
	require.NoError(t, h.ReclaimModuleController(c))

	require.True(t, raced, "the test never interleaved a rebind with the owner check")
	assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	// The simulated rebind shares the request's transaction, so it rolls back
	// with the rejected reclaim. What matters is that the stale check did not
	// land its write: without the owner guard this ends 200 with the row on
	// module api 2.
	controller := loadController(t, db, controllerID)
	assert.NotEqual(t, uint(2), *controller.ModuleApiID, "the stale owner check must not rebind the row")
	assert.Equal(t, "old-ns/old-deployment", *controller.DeploymentName)
}
