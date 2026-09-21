package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	zap "go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	api "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// Sequential per-id lookups cost perfEventCount * perfModuleLatency.
// Parallel or batched lookups stay under perfLatencyCeiling.
const (
	// perfEventCount is the event count in the enrich workload.
	perfEventCount = 20
	// perfModuleLatency is the delay the fake module injects per request.
	perfModuleLatency = 200 * time.Millisecond
	// perfLatencyCeiling is the p95 budget for one enrich.
	perfLatencyCeiling = 2 * time.Second
	// perfIterations is the number of timed enrich samples.
	perfIterations = 5
)

// setupEventsPerfHarness starts a delayed fake module, seeds sqlite with a
// Widget registry and events, and returns an enrich function over that data.
func setupEventsPerfHarness(tb testing.TB) (
	db *gorm.DB,
	enrich func() error,
	requestCount *int64,
	teardown func(),
) {
	tb.Helper()

	// fake module GET; the sleep makes sequential per-id lookups miss the ceiling
	var counter int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&counter, 1)
		time.Sleep(perfModuleLatency)
		// use the last path segment as the object ID
		segs := strings.Split(strings.TrimSuffix(r.URL.Path, "/"), "/")
		idStr := segs[len(segs)-1]
		row := map[string]interface{}{"ID": idStr, "Name": "widget-" + idStr}
		body := map[string]interface{}{
			"Data":   []interface{}{row},
			"Status": map[string]interface{}{"Code": 200, "Message": "OK"},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))

	// open an in-memory sqlite database for the registry and events
	d, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(tb, err)
	require.NoError(tb, d.AutoMigrate(
		&api.Event{},
		&api.ModuleApi{},
		&api.ModuleApiRoute{},
		&api.ModuleObject{},
	))

	// strip the scheme; the request client prepends http or https from TLS config
	endpoint := strings.TrimPrefix(server.URL, "http://")

	// skip create hooks; registry rows are fixtures, not API traffic
	skipHooks := d.Session(&gorm.Session{SkipHooks: true})
	modApi := &api.ModuleApi{
		Name:         util.Ptr("widget-module"),
		Core:         util.Ptr(false),
		ApiNamespace: util.Ptr("example.com"),
		Endpoint:     util.Ptr(endpoint),
	}
	require.NoError(tb, skipHooks.Create(modApi).Error)
	obj := &api.ModuleObject{
		Name:        util.Ptr("Widget"),
		Version:     util.Ptr("v0"),
		ModuleApiID: modApi.ID,
	}
	require.NoError(tb, skipHooks.Create(obj).Error)
	route := &api.ModuleApiRoute{
		Path:        util.Ptr("/example-com/v0/widgets"),
		ModuleApiID: modApi.ID,
	}
	require.NoError(tb, skipHooks.Create(route).Error)
	// insert the join row; association create would reject the existing object
	require.NoError(tb, skipHooks.Exec(
		"INSERT INTO v0_module_api_routes_module_objects (module_api_route_id, module_object_id) VALUES (?, ?)",
		route.ID, obj.ID,
	).Error)

	// seed one event per widget ID
	now := time.Now()
	seeds := make([]api.Event, perfEventCount)
	for i := 0; i < perfEventCount; i++ {
		widgetID := uint(i + 1)
		e := &api.Event{
			Reason:              util.Ptr(fmt.Sprintf("R%d", i)),
			Note:                util.Ptr("n"),
			Type:                util.Ptr("Normal"),
			Count:               util.Ptr(uint(1)),
			EventTime:           &now,
			LastObservedTime:    &now,
			ReportingController: util.Ptr("test"),
			ObjectType:          util.Ptr("example.com/v0.Widget"),
			ObjectID:            util.Ptr(widgetID),
		}
		require.NoError(tb, d.Session(&gorm.Session{SkipHooks: true}).Create(e).Error)
		seeds[i] = *e
	}

	logger := zap.NewNop()

	// copy seeds and clear ObjectName so a prior resolve cannot skip the lookup
	enrich = func() error {
		fresh := make([]api.Event, len(seeds))
		copy(fresh, seeds)
		for i := range fresh {
			fresh[i].ObjectName = nil
		}
		return enrichEventsWithObjectInfo(context.Background(), d, fresh, logger)
	}
	teardown = func() { server.Close() }
	return d, enrich, &counter, teardown
}

// BenchmarkGetEventsWithFakeModules measures event-name enrich against a
// delayed fake module and reports p50 and p95.
func BenchmarkGetEventsWithFakeModules(b *testing.B) {
	_, enrich, _, teardown := setupEventsPerfHarness(b)
	defer teardown()

	durations := make([]time.Duration, 0, b.N)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		if err := enrich(); err != nil {
			b.Fatalf("enrich failed: %v", err)
		}
		durations = append(durations, time.Since(start))
	}
	b.StopTimer()

	// report p50 and p95 of enrich duration
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p50 := durations[percentileIndex(len(durations), 50)]
	p95 := durations[percentileIndex(len(durations), 95)]
	b.ReportMetric(float64(p50.Milliseconds()), "p50-ms")
	b.ReportMetric(float64(p95.Milliseconds()), "p95-ms")
}

// TestGetEventsEnrich_LatencyCeiling rejects a sequential per-id module
// lookup whose p95 exceeds perfLatencyCeiling.
func TestGetEventsEnrich_LatencyCeiling(t *testing.T) {
	_, enrich, _, teardown := setupEventsPerfHarness(t)
	defer teardown()

	// run once before timing to prime the database path
	require.NoError(t, enrich())

	// collect timed enrich samples
	durations := make([]time.Duration, perfIterations)
	for i := 0; i < perfIterations; i++ {
		start := time.Now()
		require.NoError(t, enrich())
		durations[i] = time.Since(start)
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p95 := durations[percentileIndex(len(durations), 95)]

	// fail when p95 exceeds the sequential-lookup budget
	if p95 > perfLatencyCeiling {
		t.Fatalf(
			"events enrich p95 %s exceeds ceiling %s (workload: %d events, %s per module lookup); "+
				"check for sequential per-id fan-out in module name resolution",
			p95, perfLatencyCeiling, perfEventCount, perfModuleLatency,
		)
	}
}

// percentileIndex returns the sorted-slice index for pct over n samples,
// clamped to [0, n-1].
func percentileIndex(n, pct int) int {
	if n <= 0 {
		return 0
	}
	i := n * pct / 100
	if i >= n {
		i = n - 1
	}
	return i
}
