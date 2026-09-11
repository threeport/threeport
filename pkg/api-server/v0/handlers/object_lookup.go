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

// moduleLookupOverallTimeout is the budget for one module HTTP lookup.
// A slow or unreachable module must not stall the enclosing response.
const moduleLookupOverallTimeout = 10 * time.Second

// moduleLookupMaxConcurrency is the cap on parallel module HTTP requests.
// A large ID list must not exhaust file descriptors or overload a module.
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

// moduleHTTPClient is the shared HTTP client for module lookups.
// TLS is used when client certs are mounted; each request times out at 3s.
var moduleHTTPClient = func() *http.Client {
	authEnabled := moduleClientCertsMounted()
	c, err := client_lib.GetHTTPClient(authEnabled, "", "", "", "")
	if err != nil {
		// fall back to plain HTTP if the cert bundle is missing or unreadable
		c, _ = client_lib.GetHTTPClient(false, "", "", "", "")
		if c == nil {
			c = http.DefaultClient
		}
	}
	c.Timeout = 3 * time.Second
	return c
}()

// moduleClientCertsMounted reports whether client cert, key, and CA files
// are present under /etc/threeport. Missing files mean unauthenticated HTTP.
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

// GetObjectNames returns id->name for each ID of objectType.
// includeDeleted includes soft-deleted rows; unknown types return an empty map.
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

// GetObjectIDsByName returns every ID of objectType whose Name equals name,
// including soft-deleted rows. Unknown types return an error.
func GetObjectIDsByName(db *gorm.DB, objectType, name string) ([]uint, error) {
	// include soft-deleted rows so a name still resolves after delete
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

// getNamesFromModule fetches names from the owning module in ID batches.
// A response that includes an unrequested ID falls back to one GET per ID.
func getNamesFromModule(ctx context.Context, endpoint, path string, ids []uint, includeDeleted bool) (map[uint]string, error) {
	// preallocate the result map at the upper bound; ids that fail
	// lookup simply won't appear in it
	out := make(map[uint]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}

	// bound the whole lookup so a hung module cannot stall the caller
	overallCtx, cancel := context.WithTimeout(ctx, moduleLookupOverallTimeout)
	defer cancel()

	// remember requested IDs; extra IDs mean the module ignored the ids filter
	requested := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		requested[id] = struct{}{}
	}

	// split IDs so each GET stays within the list page size
	chunks := chunkIDs(ids, apiserver_lib.MaxPaginationLimitValue)

	// guard out and fallback across workers
	var mu sync.Mutex
	var fallback bool

	// run chunk GETs in parallel; in-flight requests use the client timeout
	g, gctx := errgroup.WithContext(overallCtx)
	g.SetLimit(moduleLookupMaxConcurrency)

	for _, chunk := range chunks {
		chunk := chunk

		// stop enqueueing once the overall timeout fires
		if gctx.Err() != nil {
			break
		}

		// fetch one chunk; workers share out and fallback under mu
		g.Go(func() error {
			// skip the request if cancelled while queued behind the cap
			if gctx.Err() != nil {
				return nil
			}

			// build the batched list URL for this chunk
			url := buildBulkListURL(endpoint, path, chunk, includeDeleted)

			// GET the chunk from the module; a failed chunk is skipped
			resp, err := client_lib.GetResponse(
				moduleHTTPClient,
				url,
				http.MethodGet,
				new(bytes.Buffer),
				map[string]string{},
				http.StatusOK,
			)

			// skip a failed chunk so other IDs can still resolve
			if err != nil {
				return nil
			}
			if resp == nil {
				return nil
			}

			// collect names for requested IDs; extra IDs trigger per-ID fallback
			for _, item := range resp.Data {
				row, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				id, recognized, err := parseRowID(row["ID"])
				if err != nil || !recognized {
					continue
				}
				// an unrequested ID means the module ignored the ids filter
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

	// drain in-flight chunks; workers never return an error
	_ = g.Wait()

	// ids filter was ignored; discard the bulk result and fetch per ID
	if fallback {
		return getNamesFromModulePerID(overallCtx, endpoint, path, ids, includeDeleted)
	}
	return out, nil
}

// chunkIDs splits ids into slices of at most size. A non-positive size
// returns ids as a single chunk so the loop cannot run forever.
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

// buildBulkListURL builds a list GET with ids and limit set to the chunk
// so pagination cannot truncate the result.
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

// getNamesFromModulePerID fetches one object per ID from the module.
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

		// fetch one ID; a miss leaves that ID out of the result
		g.Go(func() error {
			// skip the request if cancelled while queued behind the cap
			if gctx.Err() != nil {
				return nil
			}

			// GET one object by ID
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

// getIDsFromModuleByName returns every ID whose Name equals name from the
// module, including soft-deleted rows.
func getIDsFromModuleByName(endpoint, path, objectType, name string) ([]uint, error) {
	// list by name, including soft-deleted rows
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
