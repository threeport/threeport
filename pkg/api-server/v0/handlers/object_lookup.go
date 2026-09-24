package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	gorm "gorm.io/gorm"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
)

// parseRowID extracts a uint ID from a JSON-decoded row's ID field.
// json.Unmarshal into interface{} produces float64 by default; UseNumber()
// produces json.Number. Returns recognized=false for any other shape.
func parseRowID(idValue interface{}) (id uint, recognized bool, err error) {
	switch v := idValue.(type) {
	case float64:
		return uint(v), true, nil
	case json.Number:
		i, parseErr := v.Int64()
		if parseErr != nil {
			return 0, true, parseErr
		}
		return uint(i), true, nil
	}
	return 0, false, nil
}

// moduleHTTPClient is the HTTP client used for module API dispatch.
// Module api-servers don't currently support server-side TLS, so we
// always use plain HTTP regardless of threeport-core's auth-enabled
// setting; the call is in-cluster service-to-service. A short timeout
// caps cross-module lookups so a misconfigured/unreachable module
// doesn't stall response formatting; the caller falls back to id-only
// when the lookup fails.
var moduleHTTPClient = func() *http.Client {
	c, err := client_lib.GetHTTPClient(false, "", "", "", "")
	if err != nil {
		c = http.DefaultClient
	}
	c.Timeout = 3 * time.Second
	return c
}()

// GetObjectNames returns id->name for each id of the given object type,
// dispatching to core SQL or the owning module's API as needed. Returns
// an empty map if the type has no resolver. includeDeleted=true includes
// soft-deleted rows.
func GetObjectNames(db *gorm.DB, objectType string, ids []uint, includeDeleted bool) (map[uint]string, error) {
	// nothing to look up
	if len(ids) == 0 {
		return map[uint]string{}, nil
	}

	// try the core SQL resolver first; most types live in core, so return
	// early if found there rather than fan out to a module HTTP lookup
	names, err := apiserver_lib.GetCoreObjectNamesByIDs(db, objectType, ids, includeDeleted)
	if err == nil {
		return names, nil
	}

	// a non-"unknown core type" error is a real failure; surface it
	if !errors.Is(err, apiserver_lib.ErrUnknownCoreType) {
		return nil, err
	}

	// core doesn't know this type; look up which module owns it
	endpoint, path, err := apiserver_lib.GetModuleRouteForType(db, objectType)
	if err != nil {
		return nil, err
	}

	// no module owns it either; return empty so callers can degrade
	// gracefully rather than failing the whole response
	if endpoint == "" {
		return map[uint]string{}, nil
	}

	// dispatch to the owning module's CRUD endpoint
	return getNamesFromModule(endpoint, path, ids, includeDeleted)
}

// GetObjectIDsByName returns the IDs of all objects with the given name
// for the given object type, dispatching to core SQL or the owning
// module's API as needed. Returns an empty slice if the type has no
// resolver or no objects match. Name uniqueness is not enforced at
// the database level, so a single name can legitimately resolve to
// multiple ids.
func GetObjectIDsByName(db *gorm.DB, objectType, name string) ([]uint, error) {
	// try the core SQL resolver first; return early if the type is core
	ids, err := apiserver_lib.GetCoreObjectIDsByName(db, objectType, name)
	if err == nil {
		return ids, nil
	}

	// a non-"unknown core type" error is a real failure; surface it
	if !errors.Is(err, apiserver_lib.ErrUnknownCoreType) {
		return nil, err
	}

	// core doesn't know this type; look up which module owns it
	endpoint, path, err := apiserver_lib.GetModuleRouteForType(db, objectType)
	if err != nil {
		return nil, err
	}

	// no module owns it either; this resolver has no fallback so the
	// caller needs a hard error rather than a soft empty
	if endpoint == "" {
		return nil, fmt.Errorf("object type %q not owned by core or any registered module", objectType)
	}

	// dispatch to the owning module's CRUD endpoint
	return getIDsFromModuleByName(endpoint, path, objectType, name)
}

// getNamesFromModule fetches each id's object Name from the module API,
// skipping ids whose lookups fail.
func getNamesFromModule(endpoint, path string, ids []uint, includeDeleted bool) (map[uint]string, error) {
	// preallocate the result map at the upper bound; ids that fail
	// lookup simply won't appear in it
	out := make(map[uint]string, len(ids))

	// when the caller wants soft-deleted rows too, ask the module
	// to bypass its delete filter via the shared query param
	suffix := ""
	if includeDeleted {
		suffix = "?" + apiserver_lib.QueryParamIncludeDeleted + "=true"
	}

	// one GET per id; the module API doesn't have a batch by-ids
	// endpoint, so this is a fan-out of N requests
	for _, id := range ids {
		// build the per-id URL. e.g.
		// "threeport-widget-api-server.threeport-control-plane.svc.cluster.local/example-com/v0/widgets/42"
		// (GetResponse below prepends the http(s):// scheme based on TLS config)
		url := fmt.Sprintf("%s%s/%d%s", endpoint, path, id, suffix)

		// dispatch via the shared module HTTP client
		resp, err := client_lib.GetResponse(
			moduleHTTPClient,
			url,
			http.MethodGet,
			new(bytes.Buffer),
			map[string]string{},
			http.StatusOK,
		)

		// transport or non-200 error - skip this id rather than
		// failing the whole batch; the caller renders id-only for
		// missing names
		if err != nil {
			continue
		}

		// the module response wraps the row in a Data array. the
		// lookup is by id (primary key) so there is at most one row,
		// picked out here as a generic map.
		if resp == nil || len(resp.Data) == 0 {
			continue
		}
		row, ok := resp.Data[0].(map[string]interface{})
		if !ok {
			continue
		}

		// only record entries that actually have a non-empty Name;
		// otherwise drop and let the caller fall back to id-only
		if name, ok := row["Name"].(string); ok && name != "" {
			out[id] = name
		}
	}
	return out, nil
}

// getIDsFromModuleByName issues a name-filtered GET to the module API
// and returns every matching row's ID. An empty result is returned as
// an empty slice with no error.
func getIDsFromModuleByName(endpoint, path, objectType, name string) ([]uint, error) {
	// build the name-filtered list URL. e.g.
	// "threeport-widget-api-server.threeport-control-plane.svc.cluster.local/example-com/v0/widgets?name=my-widget"
	// (GetResponse below prepends the http(s):// scheme based on TLS config)
	url := fmt.Sprintf("%s%s?name=%s", endpoint, path, name)

	// dispatch via the shared module HTTP client
	resp, err := client_lib.GetResponse(
		moduleHTTPClient,
		url,
		http.MethodGet,
		new(bytes.Buffer),
		map[string]string{},
		http.StatusOK,
	)
	if err != nil {
		return nil, fmt.Errorf("module lookup of %s by name failed: %w", objectType, err)
	}

	// no rows is a legitimate empty result; let the caller decide how
	// to render an empty name (typically id-only)
	if resp == nil || len(resp.Data) == 0 {
		return []uint{}, nil
	}

	// collect every row's ID
	ids := make([]uint, 0, len(resp.Data))
	for _, item := range resp.Data {
		row, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		id, recognized, err := parseRowID(row["ID"])
		if err != nil {
			return nil, fmt.Errorf("invalid ID for %s: %w", objectType, err)
		}
		if recognized {
			ids = append(ids, id)
		}
	}

	return ids, nil
}
