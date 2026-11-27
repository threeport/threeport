package v0

import (
	"fmt"

	gorm "gorm.io/gorm"

	apilib "github.com/threeport/threeport/pkg/api/lib/v0"
)

// GetModuleRouteForType returns the endpoint and CRUD path the owning
// module exposes for a given object type. The input must be the
// qualified form "<api-namespace>/<version>.<TypeName>". Returns empty
// strings for inputs that don't parse as qualified or don't match a
// registered module.
//
// For an input of "example.com/v0.Widget", returns:
//
//	endpoint = "threeport-widget-api-server.threeport-control-plane.svc.cluster.local"
//	path     = "/example-com/v0/widgets"
//
// The endpoint is the bare in-cluster DNS as stored in
// v0_module_apis.endpoint - the HTTP scheme is prepended by the
// request client based on TLS configuration.
func GetModuleRouteForType(db *gorm.DB, objectType string) (string, string, error) {
	// fresh session so accumulated clauses from the caller's db don't
	// leak into this lookup
	db = apilib.NewCleanSession(db)

	// unqualified inputs aren't module types; caller falls through to a
	// graceful empty result
	namespace, version, typeName, ok := apilib.ParseQualifiedType(objectType)
	if !ok {
		return "", "", nil
	}

	// start from the routes table; we'll JOIN outward to filter by
	// module, type, and version.
	//   rows so far: every row in v0_module_api_routes
	//   ex. route(id=1, path="/example-com/v0/widgets",       module_api_id=5)
	//       route(id=2, path="/example-com/widgets/versions", module_api_id=5)
	//       ...rows from other modules registered in this control plane
	q := db.Table("v0_module_api_routes AS route")

	// pull in the parent ModuleApi row so its endpoint and namespace
	// are queryable.
	//   rows so far: every (route, module_api) pair
	//   ex. (route.path="/example-com/v0/widgets",
	//        module_api.api_namespace="example.com",
	//        module_api.endpoint="threeport-widget-api-server.threeport-control-plane.svc.cluster.local",
	//        module_api.core=false)
	//       (route.path="/example-com/widgets/versions",
	//        module_api.api_namespace="example.com",
	//        module_api.endpoint="threeport-widget-api-server.threeport-control-plane.svc.cluster.local",
	//        module_api.core=false)
	//       ...similarly for any other registered modules
	q = q.Joins("JOIN v0_module_apis AS module_api ON module_api.id = route.module_api_id")

	// keep only routes owned by the named non-core module
	// (core=false skips the threeport core API itself).
	//   rows so far: (route, module_api) pairs for "example.com" only
	//   ex. (route.path="/example-com/v0/widgets",
	//        module_api.endpoint="threeport-widget-api-server.threeport-control-plane.svc.cluster.local")
	//       (route.path="/example-com/widgets/versions",
	//        module_api.endpoint="threeport-widget-api-server.threeport-control-plane.svc.cluster.local")
	q = q.Where("module_api.api_namespace = ? AND module_api.core = false", namespace)

	// follow the ModuleApiRoute<->ModuleObject junction; the m2m is needed
	// because one route can in principle serve multiple registered types.
	//   rows so far: (route, module_api, link) triples for "example.com"
	//   ex. (route.path="/example-com/v0/widgets",       link.module_object_id=42)
	//       (route.path="/example-com/widgets/versions", link.module_object_id=42)
	q = q.Joins("JOIN v0_module_api_routes_module_objects AS link ON link.module_api_route_id = route.id")

	// land on the registered type's row, where its name and version live.
	//   rows so far: (route, module_api, link, object) for every type on
	//   "example.com"'s routes
	//   ex. (route.path="/example-com/v0/widgets",
	//        object.name="Widget", object.version="v0")
	//       (route.path="/example-com/widgets/versions",
	//        object.name="Widget", object.version="v0")
	q = q.Joins("JOIN v0_module_objects AS object ON object.id = link.module_object_id")

	// keep only the specific (name, version) the caller asked for.
	//   rows so far: routes serving "example.com"'s "Widget" at "v0"
	//   (typically 2: the CRUD path and the /versions discovery path)
	//   ex. (route.path="/example-com/v0/widgets",
	//        object.name="Widget", object.version="v0")
	//       (route.path="/example-com/widgets/versions",
	//        object.name="Widget", object.version="v0")
	q = q.Where("object.name = ? AND object.version = ?", typeName, version)

	// each registered type gets two routes: a CRUD path and a /versions
	// discovery path. This query wants the CRUD endpoint, so exclude the
	// discovery one by path suffix.
	//   rows so far: 1 - the CRUD route for "example.com/v0.Widget"
	//   ex. (route.path="/example-com/v0/widgets",
	//        module_api.endpoint="threeport-widget-api-server.threeport-control-plane.svc.cluster.local")
	q = q.Where("route.path NOT LIKE ?", "%/versions")

	// exclude soft-deleted rows on every aliased table; raw .Table()
	// and .Joins() bypass gorm's automatic deleted_at filter, so the
	// predicate has to be explicit per alias
	q = q.Where(LiveRowsFilter("route", "module_api", "object"))

	var result struct {
		Path     string
		Endpoint string
	}
	if err := q.Select("route.path, module_api.endpoint").Limit(1).Scan(&result).Error; err != nil {
		return "", "", fmt.Errorf("failed to look up module route for %s: %w", objectType, err)
	}
	if result.Path == "" || result.Endpoint == "" {
		return "", "", nil
	}
	return result.Endpoint, result.Path, nil
}

// GetObjectTypes returns every fully qualified type that the given bare kind name
// could refer to. The returned slice contains entries from both
// modules and the core registry, merged - callers that need to narrow
// further can pass the result through FilterQualifiedTypes.
//
// Cross-namespace duplicates are not surfaced as an error: if "Widget"
// is registered in both core and a module, the caller gets every
// matching fully qualified type and decides how to disambiguate (typically by reading
// the namespace-qualified form displayed alongside each result).
func GetObjectTypes(db *gorm.DB, bareKind string) ([]string, error) {
	out := []string{}

	// collect module registrations of the bare kind via the type+namespace
	// table join. This is the source of truth for module-owned types.
	type row struct {
		Namespace string
		Version   string
	}
	var rows []row

	// start from the registered objects table; JOIN outward to get the
	// namespace each registration belongs to.
	//   rows so far: every row in v0_module_objects
	//   ex. object(name="Widget",  version="v0", module_api_id=5)
	//       object(name="Gadget",  version="v0", module_api_id=5)
	//       ...rows from other modules registered in this control plane
	q := db.Table("v0_module_objects AS object")

	// pull in the parent ModuleApi row so its namespace is queryable.
	//   rows so far: every (object, module_api) pair
	//   ex. (object.name="Widget",
	//        module_api.api_namespace="example.com",
	//        module_api.core=false)
	//       (object.name="Gadget",
	//        module_api.api_namespace="example.com",
	//        module_api.core=false)
	//       ...similarly for any other registered modules
	q = q.Joins("JOIN v0_module_apis AS module_api ON module_api.id = object.module_api_id")

	// keep only registrations of the bare kind, owned by non-core modules
	// (core=false skips the threeport core API itself - core types come
	// from the in-memory registry below).
	//   rows so far: (object, module_api) pairs for "Widget" only
	//   ex. (object.name="Widget", object.version="v0",
	//        module_api.api_namespace="example.com")
	//       (object.name="Widget", object.version="v1",
	//        module_api.api_namespace="example.com")
	q = q.Where("object.name = ? AND module_api.core = false", bareKind)

	// exclude soft-deleted rows; raw .Table() / .Joins() bypass gorm's
	// automatic deleted_at filter, so the predicate is explicit per alias
	q = q.Where(LiveRowsFilter("object", "module_api"))

	if err := q.Select("module_api.api_namespace AS namespace, object.version AS version").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to resolve object type for %q: %w", bareKind, err)
	}

	// build the fully qualified type for each module registration and aggregate into out
	for _, r := range rows {
		out = append(out, fmt.Sprintf("%s/%s.%s", r.Namespace, r.Version, bareKind))
	}

	// add core registrations, if any. core types live in an in-memory
	// registry populated at startup rather than v0_module_objects.
	if obj, ok := ObjectVersions[bareKind]; ok {
		for _, v := range obj.Versions {
			out = append(out, fmt.Sprintf("%s/%s.%s", apilib.CoreApiNamespace, v, bareKind))
		}
	}

	return out, nil
}

// FilterQualifiedTypes narrows the type list to entries matching the
// optional namespace and/or version. Empty filter values match
// anything. fully qualified types are "<namespace>/<version>.<TypeName>".
func FilterQualifiedTypes(qualifiedTypes []string, namespace, version string) []string {
	// no filters supplied - everything passes through unchanged
	if namespace == "" && version == "" {
		return qualifiedTypes
	}

	// allocate a fresh backing array sized for the worst case (all
	// entries pass); the [:0:0] form keeps cap separate from the
	// input slice so we don't accidentally overwrite the caller's data
	out := qualifiedTypes[:0:0]

	for _, qt := range qualifiedTypes {
		// parse the fully qualified type into parts so we can match on namespace and
		// version independently
		ns, ver, _, ok := apilib.ParseQualifiedType(qt)
		if !ok {
			// malformed fully qualified type; drop rather than fail the whole query
			continue
		}

		// drop entries whose namespace doesn't match the filter (when set)
		if namespace != "" && ns != namespace {
			continue
		}

		// drop entries whose version doesn't match the filter (when set)
		if version != "" && ver != version {
			continue
		}

		// passed all active filters - keep it
		out = append(out, qt)
	}
	return out
}
