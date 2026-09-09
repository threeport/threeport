package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	gorm "gorm.io/gorm"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
)

// moduleLookupOverallTimeout caps the total wall-clock time spent
// resolving a batch of module names, keeping a slow or unreachable
// module from stalling the enclosing response beyond this bound.
const moduleLookupOverallTimeout = 10 * time.Second

// moduleLookupMaxConcurrency bounds the fan-out of concurrent module
// HTTP GETs so a large id list can't exhaust file descriptors or
// swamp the module api-server.
const moduleLookupMaxConcurrency = 8

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
// Module Services expose the module api-server on port 443 with mTLS
// when the control plane runs with auth enabled, and on port 80 with
// plain HTTP when auth is disabled. The client picks TLS + client
// certificate when /etc/threeport client credentials are mounted (the
// same secret the rest-api process reads for its own server cert) and
// falls back to plain HTTP otherwise. A short timeout caps cross-module
// lookups so a misconfigured or unreachable module doesn't stall
// response formatting; the caller falls back to id-only when the
// lookup fails.
var moduleHTTPClient = func() *http.Client {
	authEnabled := moduleClientCertsMounted()
	c, err := client_lib.GetHTTPClient(authEnabled, "", "", "", "")
	if err != nil {
		// fall back to a plain-http client so a missing or unreadable
		// cert bundle doesn't take down every module name resolution
		c, _ = client_lib.GetHTTPClient(false, "", "", "", "")
		if c == nil {
			c = http.DefaultClient
		}
	}
	c.Timeout = 3 * time.Second
	return c
}()

// moduleClientCertsMounted reports whether the mTLS client-cert bundle
// this process would use to reach module Services is present on disk
// at the standard /etc/threeport mount points.
func moduleClientCertsMounted() bool {
	configDir := "/etc/threeport"
	if _, err := os.Stat(filepath.Join(configDir, "cert", "tls.crt")); err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(configDir, "cert", "tls.key")); err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(configDir, "ca", "tls.crt")); err != nil {
		return false
	}
	return true
}

// GetObjectNames returns id->name for each id of the given object type,
// dispatching to core SQL or the owning module's API as needed. Returns
// an empty map if the type has no resolver. includeDeleted=true includes
// soft-deleted rows. ctx bounds the overall lookup so a slow module
// endpoint can't stall response formatting.
func GetObjectNames(ctx context.Context, db *gorm.DB, objectType string, ids []uint, includeDeleted bool) (map[uint]string, error) {
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
	return getNamesFromModule(ctx, endpoint, path, ids, includeDeleted)
}

// GetObjectIDsByName returns the IDs of all objects with the given name
// for the given object type, dispatching to core SQL or the owning
// module's API as needed. Returns an empty slice if the type has no
// resolver or no objects match. Name uniqueness is not enforced at
// the database level, so a single name can legitimately resolve to
// multiple ids.
//
// Soft-deleted objects are included. An object's event history outlives
// the object, so a name that resolves to a deleted row still names the
// subject those events belong to, and dropping it would answer a name
// filter with fewer events than the same subject's id filter returns.
func GetObjectIDsByName(db *gorm.DB, objectType, name string) ([]uint, error) {
	// try the core SQL resolver first; return early if the type is core.
	// Unscoped lifts gorm's deleted_at predicate so a deleted subject
	// still resolves.
	ids, err := apiserver_lib.GetCoreObjectIDsByName(db.Unscoped(), objectType, name)
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

// getNamesFromModule fetches id->name for every id from the module API
// in one list call per chunk of at most apiserver_lib.MaxPaginationLimitValue
// ids, filtered server-side via ?ids=<csv>. Modules that predate the ids
// filter still return their default page rather than an id-scoped result;
// that is detected here and the caller falls back to the per-id path.
// Concurrency across chunks is capped at moduleLookupMaxConcurrency and
// the whole fan-out is bounded by moduleLookupOverallTimeout (or an
// earlier deadline on ctx).
func getNamesFromModule(ctx context.Context, endpoint, path string, ids []uint, includeDeleted bool) (map[uint]string, error) {
	// preallocate the result map at the upper bound; ids that fail
	// lookup simply won't appear in it
	out := make(map[uint]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}

	// derive an overall deadline; context propagation keeps whichever
	// of the parent or derived deadline fires first
	overallCtx, cancel := context.WithTimeout(ctx, moduleLookupOverallTimeout)
	defer cancel()

	// build the requested-id set for feature-detect. A module that
	// hasn't picked up the ids filter returns its default page, which
	// may include ids we didn't ask for; a request that included ids=
	// AND respected it can only return a subset of the requested ids.
	requested := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		requested[id] = struct{}{}
	}

	// chunk the id list at the module's per-request page limit so a
	// large blocking-delete lookup still fits inside one round trip
	// per chunk
	chunks := chunkIDs(ids, apiserver_lib.MaxPaginationLimitValue)

	// guards concurrent writes to out and to the fallback trigger
	var mu sync.Mutex
	var fallback bool

	// bound fan-out with errgroup's SetLimit; the per-request cap
	// still comes from moduleHTTPClient.Timeout, so a single slow
	// endpoint can't hold a worker past that bound
	g, gctx := errgroup.WithContext(overallCtx)
	g.SetLimit(moduleLookupMaxConcurrency)

	for _, chunk := range chunks {
		chunk := chunk

		// stop dispatching once the overall context is done; already
		// in-flight requests still complete under the shared client's
		// per-request timeout
		if gctx.Err() != nil {
			break
		}

		g.Go(func() error {
			// short-circuit if the overall deadline fired while we
			// were queued behind the concurrency cap
			if gctx.Err() != nil {
				return nil
			}

			// build the chunked list URL. e.g.
			// "threeport-widget-api-server.threeport-control-plane.svc.cluster.local/example-com/v0/widgets?ids=1,2,3&limit=3"
			// (GetResponse below prepends the http(s):// scheme based on TLS config)
			url := buildBulkListURL(endpoint, path, chunk, includeDeleted)

			// dispatch via the shared module HTTP client
			resp, err := client_lib.GetResponse(
				moduleHTTPClient,
				url,
				http.MethodGet,
				new(bytes.Buffer),
				map[string]string{},
				http.StatusOK,
			)

			// transport or non-200 error; skip this chunk rather
			// than failing the whole batch, the caller renders
			// id-only for missing names
			if err != nil {
				return nil
			}
			if resp == nil {
				return nil
			}

			// walk the returned rows; short-circuit into the
			// fallback path if any row's id isn't one we asked
			// for, since that means the module ignored ids=
			for _, item := range resp.Data {
				row, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				id, recognized, err := parseRowID(row["ID"])
				if err != nil || !recognized {
					continue
				}
				if _, ok := requested[id]; !ok {
					mu.Lock()
					fallback = true
					mu.Unlock()
					return nil
				}
				if name, ok := row["Name"].(string); ok && name != "" {
					mu.Lock()
					out[id] = name
					mu.Unlock()
				}
			}
			return nil
		})
	}

	// worker goroutines never return non-nil, so Wait always returns
	// nil; the call drains all outstanding workers before returning
	_ = g.Wait()

	// a module that ignored the ids filter poisoned this batch's
	// name map; discard the partial result and rebuild via per-id
	// GETs so the caller still gets a usable id->name map
	if fallback {
		return getNamesFromModulePerID(overallCtx, endpoint, path, ids, includeDeleted)
	}
	return out, nil
}

// chunkIDs splits ids into contiguous slices of at most size ids each.
// A non-positive size returns a single chunk holding the whole slice so
// callers never see an infinite loop under a bad limit constant.
func chunkIDs(ids []uint, size int) [][]uint {
	if size <= 0 || len(ids) <= size {
		return [][]uint{ids}
	}
	chunks := make([][]uint, 0, (len(ids)+size-1)/size)
	for start := 0; start < len(ids); start += size {
		end := start + size
		if end > len(ids) {
			end = len(ids)
		}
		chunks = append(chunks, ids[start:end])
	}
	return chunks
}

// buildBulkListURL formats a module list URL that filters by id set. Limit
// is pinned to the chunk size so the module returns every requested row in
// one page.
func buildBulkListURL(endpoint, path string, ids []uint, includeDeleted bool) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatUint(uint64(id), 10)
	}
	url := fmt.Sprintf(
		"%s%s?%s=%s&%s=%d",
		endpoint,
		path,
		apiserver_lib.QueryParamIDs,
		strings.Join(parts, ","),
		apiserver_lib.QueryParamLimit,
		len(ids),
	)
	if includeDeleted {
		url += "&" + apiserver_lib.QueryParamIncludeDeleted + "=true"
	}
	return url
}

// getNamesFromModulePerID falls back to one GET per id, used when a module
// hasn't picked up the ids= list filter and returned its default page.
// Kept in place so the switch to bulk lookup degrades gracefully rather
// than silently dropping names for older modules.
func getNamesFromModulePerID(ctx context.Context, endpoint, path string, ids []uint, includeDeleted bool) (map[uint]string, error) {
	out := make(map[uint]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}

	suffix := ""
	if includeDeleted {
		suffix = "?" + apiserver_lib.QueryParamIncludeDeleted + "=true"
	}

	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(moduleLookupMaxConcurrency)

	for _, id := range ids {
		id := id

		if gctx.Err() != nil {
			break
		}

		g.Go(func() error {
			if gctx.Err() != nil {
				return nil
			}

			url := fmt.Sprintf("%s%s/%d%s", endpoint, path, id, suffix)

			resp, err := client_lib.GetResponse(
				moduleHTTPClient,
				url,
				http.MethodGet,
				new(bytes.Buffer),
				map[string]string{},
				http.StatusOK,
			)
			if err != nil {
				return nil
			}
			if resp == nil || len(resp.Data) == 0 {
				return nil
			}
			row, ok := resp.Data[0].(map[string]interface{})
			if !ok {
				return nil
			}
			if name, ok := row["Name"].(string); ok && name != "" {
				mu.Lock()
				out[id] = name
				mu.Unlock()
			}
			return nil
		})
	}

	_ = g.Wait()
	return out, nil
}

// getIDsFromModuleByName issues a name-filtered GET to the module API
// and returns every matching row's ID. An empty result is returned as
// an empty slice with no error. Soft-deleted rows are requested for the
// same reason the core resolver lifts its deleted_at predicate: a
// subject's event history outlives the subject.
func getIDsFromModuleByName(endpoint, path, objectType, name string) ([]uint, error) {
	// build the name-filtered list URL. e.g.
	// "threeport-widget-api-server.threeport-control-plane.svc.cluster.local/example-com/v0/widgets?name=my-widget&includedeleted=true"
	// (GetResponse below prepends the http(s):// scheme based on TLS config)
	url := fmt.Sprintf(
		"%s%s?name=%s&%s=true",
		endpoint,
		path,
		url.QueryEscape(name),
		apiserver_lib.QueryParamIncludeDeleted,
	)

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
