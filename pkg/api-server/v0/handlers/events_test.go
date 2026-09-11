package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	echo "github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	zap "go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// newEventsHandler returns a Handler backed by an in-memory sqlite database
// holding the event table and the registries a subject filter resolves against.
func newEventsHandler(t *testing.T) Handler {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&api.Event{},
		&api.ModuleApi{},
		&api.ModuleObject{},
		&api.ModuleApiRoute{},
		&api.AttachedObjectReference{},
		&api.Profile{},
		&api.Tier{},
	))

	// empty the process-wide name cache so the same (type, id) cannot leak a name
	moduleNameCache.mu.Lock()
	moduleNameCache.entries = map[nameCacheKey]nameCacheEntry{}
	moduleNameCache.mu.Unlock()

	return Handler{DB: db, Logger: zap.NewNop()}
}

// getEvents issues a GET with the query binder and a wrapped context the
// handler type-asserts, and returns the status and decoded events.
func getEvents(t *testing.T, h Handler, query string) (int, []api.Event) {
	t.Helper()

	e := echo.New()
	e.Binder = apiserver_lib.NewQueryBinder()
	req := httptest.NewRequest(http.MethodGet, api.PathEventsFiltered+query, nil)
	rec := httptest.NewRecorder()
	c := &apiserver_lib.CustomContext{Context: e.NewContext(req, rec)}

	require.NoError(t, h.GetEventsFiltered(c))

	if rec.Code != http.StatusOK {
		return rec.Code, nil
	}

	var body struct {
		Data []api.Event
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	return rec.Code, body.Data
}

// seedEvent inserts one Event with create hooks skipped and returns the stored row.
func seedEvent(t *testing.T, db *gorm.DB, reason, objectType string, objectID uint) *api.Event {
	t.Helper()

	now := time.Now()
	e := &api.Event{
		Reason:              util.Ptr(reason),
		Note:                util.Ptr("n"),
		Type:                util.Ptr("Normal"),
		Count:               util.Ptr(uint(1)),
		EventTime:           &now,
		LastObservedTime:    &now,
		ReportingController: util.Ptr("test"),
		ObjectType:          util.Ptr(objectType),
		ObjectID:            util.Ptr(objectID),
	}
	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(e).Error)

	return e
}

// registerModuleObject inserts a module-owned kind so a listing can resolve that
// kind to its fully qualified types.
func registerModuleObject(t *testing.T, db *gorm.DB, apiNamespace, kind, version string) {
	t.Helper()

	session := db.Session(&gorm.Session{SkipHooks: true})
	moduleApi := &api.ModuleApi{
		Name:         util.Ptr(apiNamespace),
		ApiNamespace: util.Ptr(apiNamespace),
		Core:         util.Ptr(false),
		Endpoint:     util.Ptr("module-api." + apiNamespace),
	}
	require.NoError(t, session.Create(moduleApi).Error)
	require.NoError(t, session.Create(&api.ModuleObject{
		Name:        util.Ptr(kind),
		Version:     util.Ptr(version),
		ModuleApiID: moduleApi.ID,
	}).Error)
}

// eventIDs returns the ids of events in order.
func eventIDs(events []api.Event) []uint {
	ids := make([]uint, 0, len(events))
	for _, e := range events {
		ids = append(ids, *e.ID)
	}

	return ids
}

// TestGetEvents_ObjectIdAloneFiltersAcrossSubjectTypes covers an objectid query
// that matches every subject type sharing that id.
func TestGetEvents_ObjectIdAloneFiltersAcrossSubjectTypes(t *testing.T) {
	h := newEventsHandler(t)

	widget7 := seedEvent(t, h.DB, "R0", "example.com/v0.Widget", 7)
	gadget7 := seedEvent(t, h.DB, "R1", "other.io/v0.Gadget", 7)
	seedEvent(t, h.DB, "R2", "example.com/v0.Widget", 8)

	code, events := getEvents(t, h, "?objectid=7")

	require.Equal(t, http.StatusOK, code)
	assert.ElementsMatch(t, []uint{*widget7.ID, *gadget7.ID}, eventIDs(events),
		"an id filters on its own across every subject type")
}

// TestGetEvents_ObjectIdWithObjectNameReturns400 rejects pairing objectid with
// objectname.
func TestGetEvents_ObjectIdWithObjectNameReturns400(t *testing.T) {
	h := newEventsHandler(t)
	seedEvent(t, h.DB, "R0", "example.com/v0.Widget", 7)

	code, _ := getEvents(t, h, "?objectid=7&objectname=my-widget")

	assert.Equal(t, http.StatusBadRequest, code)
}

// TestGetEvents_ObjectTypeNameNarrowsToResolvedTypes covers objecttypename plus
// objectid matching only the registered fully qualified type.
func TestGetEvents_ObjectTypeNameNarrowsToResolvedTypes(t *testing.T) {
	h := newEventsHandler(t)
	registerModuleObject(t, h.DB, "example.com", "Widget", "v0")

	resolved := seedEvent(t, h.DB, "R0", "example.com/v0.Widget", 7)
	seedEvent(t, h.DB, "R1", "other.io/v0.Widget", 7)
	seedEvent(t, h.DB, "R2", "example.com/v0.Widget", 8)

	code, events := getEvents(t, h, "?objecttypename=Widget&objectid=7")

	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, []uint{*resolved.ID}, eventIDs(events),
		"only the resolved fully qualified type matches")
}

// TestGetEvents_ExcludesSoftDeletedEvent covers a listing that omits a
// soft-deleted event.
func TestGetEvents_ExcludesSoftDeletedEvent(t *testing.T) {
	h := newEventsHandler(t)

	deleted := seedEvent(t, h.DB, "R0", "example.com/v0.Widget", 7)
	live := seedEvent(t, h.DB, "R1", "example.com/v0.Widget", 7)

	code, events := getEvents(t, h, "?objectid=7")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, events, 2, "both events are visible before the delete")

	require.NoError(t, h.DB.Session(&gorm.Session{SkipHooks: true}).
		Delete(&api.Event{}, *deleted.ID).Error)

	code, events = getEvents(t, h, "?objectid=7")

	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, []uint{*live.ID}, eventIDs(events),
		"a soft-deleted event stays out of the listing")
}

// seedProfile inserts a Profile at a chosen id so a name query can resolve
// against a core type in the same database.
func seedProfile(t *testing.T, db *gorm.DB, id uint, name string) {
	t.Helper()

	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(&api.Profile{
		Common: api.Common{ID: util.Ptr(id)},
		Name:   util.Ptr(name),
	}).Error)
}

// seedTier inserts a Tier at a chosen id so a name query can resolve against a
// core type in the same database.
func seedTier(t *testing.T, db *gorm.DB, id uint, name string) {
	t.Helper()

	require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(&api.Tier{
		Common:      api.Common{ID: util.Ptr(id)},
		Name:        util.Ptr(name),
		Criticality: util.Ptr(1),
	}).Error)
}

// TestGetEvents_ObjectNamePrefixMatchesAcrossSubjectTypes covers an objectnameprefix
// that matches across subject types and ignores a same-id type whose name does not.
func TestGetEvents_ObjectNamePrefixMatchesAcrossSubjectTypes(t *testing.T) {
	h := newEventsHandler(t)

	seedProfile(t, h.DB, 1, "myfleet2")
	seedProfile(t, h.DB, 3, "otherfleet")
	seedTier(t, h.DB, 1, "unrelated")
	seedTier(t, h.DB, 2, "myfleet2-fleet2-host2")

	fleet := seedEvent(t, h.DB, "R0", "threeport.io/v0.Profile", 1)
	child := seedEvent(t, h.DB, "R1", "threeport.io/v0.Tier", 2)
	seedEvent(t, h.DB, "R2", "threeport.io/v0.Tier", 1)
	seedEvent(t, h.DB, "R3", "threeport.io/v0.Profile", 3)

	code, events := getEvents(t, h, "?objectnameprefix=myfleet2")

	require.Equal(t, http.StatusOK, code)
	assert.ElementsMatch(t, []uint{*fleet.ID, *child.ID}, eventIDs(events),
		"a name prefix reaches every subject type whose name starts with it")
}

// TestGetEvents_ObjectNamePrefixNarrowsToObjectTypeName covers objecttypename
// plus objectnameprefix matching only that kind.
func TestGetEvents_ObjectNamePrefixNarrowsToObjectTypeName(t *testing.T) {
	h := newEventsHandler(t)
	withCoreObjectVersions(t, "Tier", "v0")

	seedProfile(t, h.DB, 1, "myfleet2")
	seedTier(t, h.DB, 2, "myfleet2-fleet2-host2")

	seedEvent(t, h.DB, "R0", "threeport.io/v0.Profile", 1)
	child := seedEvent(t, h.DB, "R1", "threeport.io/v0.Tier", 2)

	code, events := getEvents(t, h, "?objecttypename=Tier&objectnameprefix=myfleet2")

	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, []uint{*child.ID}, eventIDs(events),
		"the kind narrows the prefix to one subject type")
}

// TestGetEvents_ObjectNamePrefixMatchingNothingReturns404 covers an
// objectnameprefix that matches no object.
func TestGetEvents_ObjectNamePrefixMatchingNothingReturns404(t *testing.T) {
	h := newEventsHandler(t)

	seedProfile(t, h.DB, 1, "myfleet2")
	seedEvent(t, h.DB, "R0", "threeport.io/v0.Profile", 1)

	code, _ := getEvents(t, h, "?objectnameprefix=nosuchfleet")

	assert.Equal(t, http.StatusNotFound, code)
}

// TestGetEvents_ObjectNamePrefixRejections rejects pairing objectnameprefix with
// objectname or objectid, and a prefix carrying a character a name cannot hold.
func TestGetEvents_ObjectNamePrefixRejections(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"with objectname", "?objecttypename=Tier&objectname=myfleet2&objectnameprefix=myfleet2"},
		{"with objectid", "?objectid=1&objectnameprefix=myfleet2"},
		{"embedded star", "?objectnameprefix=my*fleet2"},
		{"leading hyphen", "?objectnameprefix=-myfleet2"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newEventsHandler(t)
			seedProfile(t, h.DB, 1, "myfleet2")
			seedEvent(t, h.DB, "R0", "threeport.io/v0.Profile", 1)

			code, _ := getEvents(t, h, test.query)

			assert.Equal(t, http.StatusBadRequest, code)
		})
	}
}

// withCoreObjectVersions sets the in-memory core type registry to one kind for
// the duration of the test, which is how a bare kind becomes a fully qualified type.
func withCoreObjectVersions(t *testing.T, kind, version string) {
	t.Helper()

	previous := apiserver_lib.ObjectVersions
	apiserver_lib.ObjectVersions = map[string]apiserver_lib.ApiObjectVersions{
		kind: {API: kind, Versions: []string{version}},
	}
	t.Cleanup(func() { apiserver_lib.ObjectVersions = previous })
}

// TestGetEvents_ObjectNameAloneMatchesAcrossSubjectTypes covers an objectname
// that matches across subject types and ignores a same-id type whose name does not.
func TestGetEvents_ObjectNameAloneMatchesAcrossSubjectTypes(t *testing.T) {
	h := newEventsHandler(t)

	seedProfile(t, h.DB, 1, "host-023421")
	seedTier(t, h.DB, 2, "host-023421")
	seedTier(t, h.DB, 1, "unrelated")

	profile := seedEvent(t, h.DB, "R0", "threeport.io/v0.Profile", 1)
	tier := seedEvent(t, h.DB, "R1", "threeport.io/v0.Tier", 2)
	seedEvent(t, h.DB, "R2", "threeport.io/v0.Tier", 1)

	code, events := getEvents(t, h, "?objectname=host-023421")

	require.Equal(t, http.StatusOK, code)
	assert.ElementsMatch(t, []uint{*profile.ID, *tier.ID}, eventIDs(events),
		"a name reaches every subject type carrying it, paired with that type's ids")
}

// TestGetEvents_ObjectNameNarrowsToObjectTypeName covers objecttypename plus
// objectname matching only that kind.
func TestGetEvents_ObjectNameNarrowsToObjectTypeName(t *testing.T) {
	h := newEventsHandler(t)
	withCoreObjectVersions(t, "Tier", "v0")

	seedProfile(t, h.DB, 1, "host-023421")
	seedTier(t, h.DB, 2, "host-023421")

	seedEvent(t, h.DB, "R0", "threeport.io/v0.Profile", 1)
	tier := seedEvent(t, h.DB, "R1", "threeport.io/v0.Tier", 2)

	code, events := getEvents(t, h, "?objecttypename=Tier&objectname=host-023421")

	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, []uint{*tier.ID}, eventIDs(events),
		"the kind narrows the name to one subject type")
}

// TestGetEvents_ObjectNameMatchingNothingReturns404 covers an objectname that
// matches no object.
func TestGetEvents_ObjectNameMatchingNothingReturns404(t *testing.T) {
	h := newEventsHandler(t)

	seedProfile(t, h.DB, 1, "host-023421")
	seedEvent(t, h.DB, "R0", "threeport.io/v0.Profile", 1)

	code, _ := getEvents(t, h, "?objectname=nosuchhost")

	assert.Equal(t, http.StatusNotFound, code)
}

// TestGetEvents_ObjectTypeNameAloneFiltersByKind covers a bare objecttypename
// that matches every id under the registered kind and excludes the same kind in another namespace.
func TestGetEvents_ObjectTypeNameAloneFiltersByKind(t *testing.T) {
	h := newEventsHandler(t)
	registerModuleObject(t, h.DB, "example.com", "Widget", "v0")

	first := seedEvent(t, h.DB, "R0", "example.com/v0.Widget", 7)
	second := seedEvent(t, h.DB, "R1", "example.com/v0.Widget", 8)
	seedEvent(t, h.DB, "R2", "other.io/v0.Widget", 7)
	seedEvent(t, h.DB, "R3", "example.com/v0.Gadget", 7)

	code, events := getEvents(t, h, "?objecttypename=Widget")

	require.Equal(t, http.StatusOK, code)
	assert.ElementsMatch(t, []uint{*first.ID, *second.ID}, eventIDs(events),
		"a bare kind filters on its own, across every id under it")
}

// TestGetEvents_ObjectTypeNameAloneUnregisteredReturns404 covers an
// objecttypename that is not registered.
func TestGetEvents_ObjectTypeNameAloneUnregisteredReturns404(t *testing.T) {
	h := newEventsHandler(t)
	seedEvent(t, h.DB, "R0", "example.com/v0.Widget", 7)

	code, _ := getEvents(t, h, "?objecttypename=NoSuchKind")

	assert.Equal(t, http.StatusNotFound, code)
}
