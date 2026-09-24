package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	api "github.com/threeport/threeport/pkg/api/v0"
)

// setupLookupTestDB returns an in-memory sqlite db with the module
// registry tables migrated. GetModuleRouteForType and GetObjectTypes
// both read from v0_module_apis / v0_module_api_routes /
// v0_module_objects (and the m2m join), so this fixture stands them
// up directly rather than going through the full api-server stack.
func setupLookupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&api.ModuleApi{},
		&api.ModuleApiRoute{},
		&api.ModuleObject{},
		// AfterCreate hooks on the registry types emit AOR rows for
		// their "requires" foreign keys, so the table must exist even
		// though we don't read from it here.
		&api.AttachedObjectReference{},
	))
	return db
}

// seedModule creates a ModuleApi for namespace and then registers a
// single (kind, version) on it via registerKindVersion. Returns the
// ModuleApi so the caller can attach additional kinds/versions.
// Uses SkipHooks so seeding bypasses the duplicate-name validator and
// the AfterCreate AOR emitters - those are unrelated to what these
// tests are exercising.
func seedModule(t *testing.T, db *gorm.DB, namespace, kind, version string) *api.ModuleApi {
	t.Helper()
	tx := db.Session(&gorm.Session{SkipHooks: true})

	endpoint := "threeport-" + namespace + "-api-server.threeport-control-plane.svc.cluster.local"
	core := false
	moduleApi := &api.ModuleApi{
		Name:         strPtr(namespace + "-api"),
		ApiNamespace: strPtr(namespace),
		Endpoint:     strPtr(endpoint),
		Core:         &core,
	}
	require.NoError(t, tx.Create(moduleApi).Error)
	registerKindVersion(t, tx, moduleApi, namespace, kind, version)
	return moduleApi
}

// registerKindVersion adds a (kind, version) registration to an
// existing ModuleApi: one ModuleObject row plus two ModuleApiRoute
// rows (CRUD and /versions discovery) joined via the m2m table.
// Production registers each version as its own ModuleObject under the
// same ModuleApi - this mirrors that shape.
func registerKindVersion(t *testing.T, tx *gorm.DB, moduleApi *api.ModuleApi, namespace, kind, version string) {
	t.Helper()
	moduleObject := &api.ModuleObject{
		Name:        strPtr(kind),
		Version:     strPtr(version),
		ModuleApiID: moduleApi.ID,
	}
	require.NoError(t, tx.Create(moduleObject).Error)

	crudPath := "/" + namespace + "/" + version + "/" + kind + "s"
	crudRoute := &api.ModuleApiRoute{
		Path:        strPtr(crudPath),
		ModuleApiID: moduleApi.ID,
	}
	require.NoError(t, tx.Create(crudRoute).Error)
	require.NoError(t, tx.Model(crudRoute).Association("ModuleObjects").Append(moduleObject))

	versionsPath := "/" + namespace + "/" + kind + "s/versions"
	versionsRoute := &api.ModuleApiRoute{
		Path:        strPtr(versionsPath),
		ModuleApiID: moduleApi.ID,
	}
	require.NoError(t, tx.Create(versionsRoute).Error)
	require.NoError(t, tx.Model(versionsRoute).Association("ModuleObjects").Append(moduleObject))
}

func strPtr(s string) *string { return &s }

// TestGetModuleRouteForType drives the JOIN-based lookup against a
// single seeded module. The happy path also covers the /versions-route
// exclusion: seedModule plants both the CRUD and /versions routes, and
// the table expects the CRUD path back.
func TestGetModuleRouteForType(t *testing.T) {
	const wantEndpoint = "threeport-example.com-api-server.threeport-control-plane.svc.cluster.local"

	cases := []struct {
		name         string
		query        string
		wantEndpoint string
		wantPath     string
	}{
		{
			name:         "happy path returns crud route, not versions route",
			query:        "example.com/v0.Widget",
			wantEndpoint: wantEndpoint,
			wantPath:     "/example.com/v0/Widgets",
		},
		{
			name:  "unknown namespace returns empty (caller degrades gracefully)",
			query: "other.com/v0.Widget",
		},
		{
			name:  "unknown version returns empty (version constraint filters)",
			query: "example.com/v9.Widget",
		},
		{
			name:  "malformed input returns empty without hitting db",
			query: "Widget",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupLookupTestDB(t)
			seedModule(t, db, "example.com", "Widget", "v0")

			endpoint, path, err := GetModuleRouteForType(db, tc.query)
			require.NoError(t, err)
			assert.Equal(t, tc.wantEndpoint, endpoint)
			assert.Equal(t, tc.wantPath, path)
		})
	}
}

// TestGetObjectTypes covers the union of module-registered (DB) and
// core-registered (in-memory ObjectVersions) types for a given bare
// kind. Each case sets up its own state via the setup function so the
// table reads as a flat list of scenarios.
func TestGetObjectTypes(t *testing.T) {
	cases := []struct {
		name       string
		kind       string
		setup      func(t *testing.T, db *gorm.DB)
		want       []string
		wantEmpty  bool
	}{
		{
			name: "module only - no core registration",
			kind: "Widget",
			setup: func(t *testing.T, db *gorm.DB) {
				seedModule(t, db, "example.com", "Widget", "v0")
			},
			want: []string{"example.com/v0.Widget"},
		},
		{
			name: "multiple versions in one module",
			kind: "Widget",
			setup: func(t *testing.T, db *gorm.DB) {
				moduleApi := seedModule(t, db, "example.com", "Widget", "v0")
				registerKindVersion(t, db.Session(&gorm.Session{SkipHooks: true}), moduleApi, "example.com", "Widget", "v1")
			},
			want: []string{"example.com/v0.Widget", "example.com/v1.Widget"},
		},
		{
			name: "core registry only - in-memory ObjectVersions",
			kind: "KubernetesWorkloadDefinition",
			setup: func(t *testing.T, db *gorm.DB) {
				withObjectVersions(t, map[string]ApiObjectVersions{
					"KubernetesWorkloadDefinition": {Versions: []string{"v0"}},
				})
			},
			want: []string{"threeport.io/v0.KubernetesWorkloadDefinition"},
		},
		{
			name: "core and module both register the kind - both surface",
			kind: "Widget",
			setup: func(t *testing.T, db *gorm.DB) {
				seedModule(t, db, "example.com", "Widget", "v0")
				withObjectVersions(t, map[string]ApiObjectVersions{
					"Widget": {Versions: []string{"v0"}},
				})
			},
			want: []string{"example.com/v0.Widget", "threeport.io/v0.Widget"},
		},
		{
			name: "unknown kind returns empty",
			kind: "Nonexistent",
			setup: func(t *testing.T, db *gorm.DB) {
				withObjectVersions(t, map[string]ApiObjectVersions{})
			},
			wantEmpty: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupLookupTestDB(t)
			tc.setup(t, db)

			got, err := GetObjectTypes(db, tc.kind)
			require.NoError(t, err)
			if tc.wantEmpty {
				assert.Empty(t, got)
				return
			}
			assert.ElementsMatch(t, tc.want, got)
		})
	}
}

// withObjectVersions swaps ObjectVersions for the duration of a test
// and restores the original at cleanup. Used by core-registry test
// cases that need a deterministic in-memory registry.
func withObjectVersions(t *testing.T, versions map[string]ApiObjectVersions) {
	t.Helper()
	prev := ObjectVersions
	ObjectVersions = versions
	t.Cleanup(func() { ObjectVersions = prev })
}

// TestFilterQualifiedTypes covers the pure-function filter applied
// after GetObjectTypes returns the union of core + module candidates.
// The two filter axes are independent and either or both may be empty.
func TestFilterQualifiedTypes(t *testing.T) {
	in := []string{
		"threeport.io/v0.Widget",
		"threeport.io/v1.Widget",
		"example.com/v0.Widget",
		"example.com/v1.Widget",
	}

	cases := []struct {
		name      string
		namespace string
		version   string
		want      []string
	}{
		{
			name: "no filter returns input unchanged",
			want: in,
		},
		{
			name:      "namespace filter",
			namespace: "example.com",
			want: []string{
				"example.com/v0.Widget",
				"example.com/v1.Widget",
			},
		},
		{
			name:    "version filter",
			version: "v0",
			want: []string{
				"threeport.io/v0.Widget",
				"example.com/v0.Widget",
			},
		},
		{
			name:      "namespace and version filter",
			namespace: "example.com",
			version:   "v1",
			want: []string{
				"example.com/v1.Widget",
			},
		},
		{
			name:      "no matches",
			namespace: "other.com",
			want:      []string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FilterQualifiedTypes(in, tc.namespace, tc.version)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestFilterQualifiedTypes_DropsMalformed drops entries that can't be
// parsed rather than failing the whole call - one bad entry from a
// stale registry shouldn't blank the result.
func TestFilterQualifiedTypes_DropsMalformed(t *testing.T) {
	in := []string{
		"example.com/v0.Widget",
		"not-a-qualified-type",
		"example.com/v1.Widget",
	}
	got := FilterQualifiedTypes(in, "example.com", "")
	assert.Equal(t, []string{
		"example.com/v0.Widget",
		"example.com/v1.Widget",
	}, got)
}

// TestLiveRowsFilter covers the helper used by raw-SQL query builders
// to compose the deleted_at IS NULL predicate per aliased table. The
// helper exists because raw .Table() / .Joins() forms bypass gorm's
// automatic soft-delete filter; the test pins its behavior for the
// shapes call sites actually use.
func TestLiveRowsFilter(t *testing.T) {
	cases := []struct {
		name    string
		aliases []string
		want    string
	}{
		{
			name:    "no aliases returns empty string",
			aliases: nil,
			want:    "",
		},
		{
			name:    "single alias",
			aliases: []string{"m"},
			want:    "m.deleted_at IS NULL",
		},
		{
			name:    "two aliases joined by AND",
			aliases: []string{"m", "r"},
			want:    "m.deleted_at IS NULL AND r.deleted_at IS NULL",
		},
		{
			name:    "bare table name (no alias) works as the alias",
			aliases: []string{"v0_attached_object_references"},
			want:    "v0_attached_object_references.deleted_at IS NULL",
		},
		{
			name:    "many aliases",
			aliases: []string{"a", "b", "c", "d"},
			want:    "a.deleted_at IS NULL AND b.deleted_at IS NULL AND c.deleted_at IS NULL AND d.deleted_at IS NULL",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, LiveRowsFilter(tc.aliases...))
		})
	}
}

// softDeleteAll directly marks every row in the given tables as
// soft-deleted via UPDATE. The ModuleApi.beforeDelete hook rejects
// deletion while routes exist, so the probe needs a way to plant
// soft-deleted state without going through the validator.
func softDeleteAll(t *testing.T, db *gorm.DB, tableNames ...string) {
	t.Helper()
	for _, tbl := range tableNames {
		require.NoError(t, db.Exec("UPDATE "+tbl+" SET deleted_at = CURRENT_TIMESTAMP").Error)
	}
}

// TestGetModuleRouteForType_ExcludesSoftDeleted verifies the lookup
// filters out soft-deleted module / route / object rows. The JOIN
// uses .Table() / .Joins() with raw strings, which bypass gorm's
// automatic deleted_at filter, so the WHERE clauses must include
// explicit `deleted_at IS NULL` predicates per alias.
func TestGetModuleRouteForType_ExcludesSoftDeleted(t *testing.T) {
	db := setupLookupTestDB(t)
	seedModule(t, db, "example.com", "Widget", "v0")

	// soft-delete every registry row directly, bypassing ModuleApi's
	// beforeDelete validator (it rejects deletion while routes exist)
	softDeleteAll(t, db,
		"v0_module_apis",
		"v0_module_api_routes",
		"v0_module_objects",
	)

	endpoint, path, err := GetModuleRouteForType(db, "example.com/v0.Widget")
	require.NoError(t, err)
	assert.Empty(t, endpoint, "soft-deleted rows must not surface in lookup")
	assert.Empty(t, path, "soft-deleted rows must not surface in lookup")
}

// TestModuleApi_NameNamespaceUnique verifies the composite unique
// index on (Name, ApiNamespace) rejects duplicate ModuleApi rows.
func TestModuleApi_NameNamespaceUnique(t *testing.T) {
	db := setupLookupTestDB(t)
	tx := db.Session(&gorm.Session{SkipHooks: true})

	core := false
	first := &api.ModuleApi{
		Name:         strPtr("widget-api"),
		ApiNamespace: strPtr("example.com"),
		Endpoint:     strPtr("ep-1"),
		Core:         &core,
	}
	require.NoError(t, tx.Create(first).Error)

	dup := &api.ModuleApi{
		Name:         strPtr("widget-api"),
		ApiNamespace: strPtr("example.com"),
		Endpoint:     strPtr("ep-2"),
		Core:         &core,
	}
	err := tx.Create(dup).Error
	require.Error(t, err, "(Name, ApiNamespace) composite unique must reject duplicates")
	assert.Contains(t, err.Error(), "UNIQUE")
}

// TestGetObjectTypes_ExcludesSoftDeleted mirrors the above for the
// kind->qualified type lookup used by `tptctl events --for kind=...`
// to expand a bare kind into candidate fully qualified types.
func TestGetObjectTypes_ExcludesSoftDeleted(t *testing.T) {
	db := setupLookupTestDB(t)
	seedModule(t, db, "example.com", "Widget", "v0")

	softDeleteAll(t, db,
		"v0_module_apis",
		"v0_module_api_routes",
		"v0_module_objects",
	)

	out, err := GetObjectTypes(db, "Widget")
	require.NoError(t, err)
	assert.Empty(t, out, "soft-deleted module rows must not surface in GetObjectTypes")
}
