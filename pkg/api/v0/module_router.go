package v0

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// The core API is the only address clients call. Module APIs register
// their routes in the database, and matching requests are proxied to
// those API servers.

// ModuleRouter is a reverse proxy from core API paths to module APIs.
// Scheme and transport are written at startup and read on later route adds.
type ModuleRouter struct {
	routes         sync.Map
	proxyScheme    string
	proxyTransport http.RoundTripper
}

// ModRouter is the process-wide module router. Persist hooks add and
// remove routes on it after startup.
var ModRouter = ModuleRouter{
	routes:         sync.Map{},
	proxyScheme:    "http",
	proxyTransport: http.DefaultTransport,
}

// InitModuleRouter loads non-core module API routes from the database,
// configures the reverse proxy, and registers it as request middleware.
func InitModuleRouter(
	db *gorm.DB,
	e *echo.Echo,
	authEnabled bool,
) error {
	// build the module proxy transport
	transport, err := moduleProxyTransport(authEnabled)
	if err != nil {
		return fmt.Errorf("failed to build module proxy transport: %w", err)
	}

	// set the proxy scheme from auth
	scheme := "http"
	if authEnabled {
		scheme = "https"
	}

	// store scheme and transport for route registration, which reads them
	ModRouter.SetProxyConfig(scheme, transport)

	// startup goes through the same reconciliation as a create or a delete.
	// Registering routes directly here would be a second rule for what the
	// router should hold, and the two would disagree the moment a path is named
	// by more than one row - a restart serving a path that reconciliation had
	// removed, or the reverse.
	if err := ReconcileModuleRoutes(db); err != nil {
		return fmt.Errorf("failed to reconcile module routes: %w", err)
	}

	// a route committed by one rest-api process is not visible to another: the
	// router is process-local and post-commit reconciliation only runs in the
	// process that handled the write. Re-reading on every proxied request would
	// put a query on the path of every core API request, not only the module
	// ones, so each process re-reads on its own instead and converges within
	// the interval.
	startModuleRouteResync(db, e.Logger)

	// register as middleware wrapping the matched or not-found handler
	e.Use(ModRouter.ServeModuleRoutes)

	return nil
}

// moduleRouteResyncRunning says whether a goroutine is re-reading the routes,
// and moduleRouteResyncInterval is how long it leaves between passes. The
// interval is a variable so a test does not have to sit through it.
var (
	moduleRouteResync         sync.Mutex
	moduleRouteResyncStop     chan struct{}
	moduleRouteResyncStopped  chan struct{}
	moduleRouteResyncInterval = 30 * time.Second
)

// startModuleRouteResync re-reads the module routes on an interval so a process
// picks up what another one committed.
//
// The router is process-local and post-commit reconciliation runs only in the
// process that handled the write, so with more than one rest-api replica a
// route created on one is not proxied by the others, and a route deleted on one
// goes on being proxied by them. Each process converges within the interval
// instead. A stock install runs a single replica, where this only re-reads what
// the process already agrees with.
//
// One goroutine per process: a second call replaces the first rather than
// adding to it, so a process cannot end up with two reading the same table.
func startModuleRouteResync(db *gorm.DB, logger echo.Logger) {
	moduleRouteResync.Lock()
	defer moduleRouteResync.Unlock()

	stopModuleRouteResyncLocked()

	// read once rather than on every pass, so nothing has to synchronise with a
	// caller that changes it
	interval := moduleRouteResyncInterval
	stop := make(chan struct{})
	stopped := make(chan struct{})
	moduleRouteResyncStop = stop
	moduleRouteResyncStopped = stopped

	go func() {
		defer close(stopped)

		// whether the previous pass failed, so a change of state is reported
		// once rather than on every interval
		failing := false

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				err := ReconcileModuleRoutes(db)

				// a failed pass is not fatal, the next one runs anyway - but
				// this is the only thing keeping a replica in step with the
				// others, so a run of failures means routing is quietly going
				// stale and has to be visible. Only the change of state is
				// reported: saying the same thing every interval would bury it.
				switch {
				case err != nil && !failing:
					failing = true
					if logger != nil {
						logger.Warnf("module route resync failing, routing may be stale: %v", err)
					}
				case err == nil && failing:
					failing = false
					if logger != nil {
						logger.Info("module route resync recovered")
					}
				}
			}
		}
	}()
}

// stopModuleRouteResync stops the resync goroutine and waits for it to finish.
// A rest-api process resyncs for as long as it runs and never calls this; it
// exists so a caller that starts one can also end it.
func stopModuleRouteResync() {
	moduleRouteResync.Lock()
	defer moduleRouteResync.Unlock()

	stopModuleRouteResyncLocked()
}

// stopModuleRouteResyncLocked is stopModuleRouteResync with the lock held.
func stopModuleRouteResyncLocked() {
	if moduleRouteResyncStop == nil {
		return
	}

	close(moduleRouteResyncStop)
	<-moduleRouteResyncStopped
	moduleRouteResyncStop = nil
	moduleRouteResyncStopped = nil
}

// ReconcileModuleRoutes brings every path the router or the database knows
// about in line with what is committed.
//
// Both sides are walked: a path the database has and the router does not is one
// another process committed, and a path the router has and the database does
// not is one another process deleted. Each is reconciled on its own, reading
// committed state as it goes, rather than from one snapshot - a snapshot would
// remove a route that committed while it was being taken.
func ReconcileModuleRoutes(db *gorm.DB) error {
	var paths []string
	if result := db.Model(&ModuleApiRoute{}).Distinct().Pluck("path", &paths); result.Error != nil {
		return fmt.Errorf("failed to query module API route paths from database: %w", result.Error)
	}

	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		seen[path] = struct{}{}
	}
	ModRouter.routes.Range(func(key, _ any) bool {
		if path, ok := key.(string); ok {
			if _, found := seen[path]; !found {
				paths = append(paths, path)
			}
		}

		return true
	})

	// every path is attempted even after one fails. A path whose module API
	// cannot be loaded fails the same way on every pass, and stopping there
	// would mean the rest are never reconciled - the router-only paths are
	// walked last, so one bad row would keep every route another process
	// deleted alive for as long as that row exists.
	var errs []error
	for _, path := range paths {
		if err := ReconcileModuleRoute(db, path); err != nil {
			errs = append(errs, fmt.Errorf("path %s: %w", path, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed to reconcile module routes: %w", errors.Join(errs...))
	}

	return nil
}

// moduleProxyHandler returns the handler that proxies a core API path to a
// module API endpoint.
//
// Startup and post-commit reconciliation both register routes, and a handler
// built differently in either place would mean the route a restart produces
// differs from the one a create produces.
func moduleProxyHandler(
	scheme string,
	transport http.RoundTripper,
	endpoint string,
) echo.HandlerFunc {
	return func(c echo.Context) error {
		// parse the module proxy target URL
		proxyUrl, err := url.Parse(fmt.Sprintf("%s://%s", scheme, endpoint))
		if err != nil {
			return fmt.Errorf("failed to parse module's proxy target URL: %w", err)
		}
		// create a reverse proxy for the module API
		proxy := httputil.NewSingleHostReverseProxy(proxyUrl)
		// use the shared module proxy transport
		proxy.Transport = transport
		// serve the proxied request
		proxy.ServeHTTP(c.Response().Writer, c.Request())

		return nil
	}
}

// moduleRouteReconcile serialises reconciliation so that concurrent operations
// on one path cannot leave the router holding what the database does not.
var moduleRouteReconcile sync.Mutex

// moduleRouteReconcileAttempts and moduleRouteReconcileWaitSeconds bound the
// retry of the reads reconciliation makes. They are variables rather than
// constants so a test does not have to wait through the delay.
var (
	moduleRouteReconcileAttempts    = 5
	moduleRouteReconcileWaitSeconds = 1
)

// pendingModuleRoutes holds paths whose reconciliation could not read the
// database, so they can be repaired.
//
// The write these paths describe has already committed by the time
// reconciliation runs, and the request that carried it cannot be replayed to
// try again: repeating a create answers 409 and repeating a delete answers 404,
// and neither reaches reconciliation. Without repair the router would hold the
// wrong thing for that path until the process restarted.
var pendingModuleRoutes sync.Map

// moduleRouteRepairRunning says whether a goroutine is draining the pending
// paths, and moduleRouteRepairWait is how long it leaves between attempts. The
// wait is a variable so a test does not have to sit through it.
var (
	moduleRouteRepairRunning atomic.Bool
	moduleRouteRepairWait    = 10 * time.Second
)

// hasPendingModuleRoutes reports whether any path is waiting to be repaired.
func hasPendingModuleRoutes() bool {
	pending := false
	pendingModuleRoutes.Range(func(any, any) bool {
		pending = true

		return false
	})

	return pending
}

// startModuleRouteRepair makes sure a goroutine is retrying the paths
// reconciliation could not read, and returns without starting a second one.
//
// Retrying on the next reconciliation is not enough on its own: nothing
// guarantees another module route is ever created or deleted, and until one is,
// the router goes on serving a path the database no longer has, or not serving
// one it does. The goroutine exits once nothing is pending, so a control plane
// that never fails a read never runs it.
func startModuleRouteRepair(db *gorm.DB) {
	if !moduleRouteRepairRunning.CompareAndSwap(false, true) {
		return
	}

	go func() {
		defer func() {
			moduleRouteRepairRunning.Store(false)
			// a path that became pending as this was exiting would otherwise
			// be left with nothing retrying it
			if hasPendingModuleRoutes() {
				startModuleRouteRepair(db)
			}
		}()

		for hasPendingModuleRoutes() {
			time.Sleep(moduleRouteRepairWait)

			moduleRouteReconcile.Lock()
			drainPendingModuleRoutes(db, "")
			moduleRouteReconcile.Unlock()
		}
	}()
}

// ReconcileModuleRoute brings the router's entry for one path in line with what
// the database holds, and is the only thing that writes to the router after
// startup.
//
// It is called after a transaction commits rather than from a persist hook. A
// hook runs inside the transaction, so a create that never commits would leave
// a live route proxying to a row that does not exist, and a delete that rolls
// back would stop serving a route whose row survives until the next restart
// rebuilds the map. Reading committed state here means the router can only ever
// hold what the database agreed to.
//
// The path is looked up rather than passed as an object because that is what
// makes this idempotent: whatever the caller did, the router ends up with the
// entry the database justifies, or with none.
func ReconcileModuleRoute(db *gorm.DB, path string) error {
	// the read and the write are held together. Two reconciliations for the
	// same path can otherwise interleave so that the one which read the older
	// state writes last - a delete removing a route a later create had already
	// committed, for instance. Serialising them means whoever writes last is
	// whoever read last. This lock is taken after the transaction has
	// committed, never across it, so a retried transaction does not hold it.
	moduleRouteReconcile.Lock()
	defer moduleRouteReconcile.Unlock()

	// repair any path a previous reconciliation could not read before doing
	// this one, since a reachable database is what they were waiting for
	drainPendingModuleRoutes(db, path)

	if err := reconcileModuleRouteLocked(db, path); err != nil {
		pendingModuleRoutes.Store(path, struct{}{})
		startModuleRouteRepair(db)

		return err
	}
	pendingModuleRoutes.Delete(path)

	return nil
}

// drainPendingModuleRoutes retries the paths an earlier reconciliation could
// not read. The caller holds the reconciliation lock.
//
// A path that fails again is left pending, and a failure is not reported: the
// caller is reconciling a different path and its own result is what the
// request turns on.
func drainPendingModuleRoutes(db *gorm.DB, except string) {
	pendingModuleRoutes.Range(func(key, _ any) bool {
		pending, ok := key.(string)
		if !ok || pending == except {
			return true
		}
		if err := reconcileModuleRouteLocked(db, pending); err == nil {
			pendingModuleRoutes.Delete(pending)
		}

		return true
	})
}

// reconcileModuleRouteLocked is the body of ReconcileModuleRoute. The caller
// holds the reconciliation lock.
//
// The reads are retried: they run after the write committed, so giving up on a
// database that is briefly unreachable would leave the router disagreeing with
// a change the client was told nothing more about.
func reconcileModuleRouteLocked(db *gorm.DB, path string) error {
	// ordered by ID, and only the first row is used. Path carries a unique
	// index among rows that are not soft deleted, so a schema created from the
	// current model holds at most one - but the migration leaves an existing
	// table alone, so a control plane upgraded into that model may not have the
	// index. Ordering makes the choice the same either way, and stable across
	// restarts, which a map iteration order is not.
	var routes []ModuleApiRoute
	if err := util.Retry(
		moduleRouteReconcileAttempts,
		moduleRouteReconcileWaitSeconds,
		func() error {
			routes = nil

			return db.Where("path = ?", path).Order("id").Find(&routes).Error
		},
	); err != nil {
		return fmt.Errorf("failed to query module API routes for path %s: %w", path, err)
	}

	// no committed row for this path: the router must not serve it. This covers
	// a delete that committed, and a create that did not.
	if len(routes) == 0 {
		ModRouter.RemoveRoute(path)

		return nil
	}

	winner := routes[0]

	var modApi ModuleApi
	if err := util.Retry(
		moduleRouteReconcileAttempts,
		moduleRouteReconcileWaitSeconds,
		func() error {
			modApi = ModuleApi{}

			return db.Where("id = ?", *winner.ModuleApiID).First(&modApi).Error
		},
	); err != nil {
		return fmt.Errorf("failed to retrieve module API for route %s: %w", path, err)
	}

	// core routes are served by this process, not proxied
	if modApi.Core != nil && *modApi.Core {
		ModRouter.RemoveRoute(path)

		return nil
	}

	scheme, transport := ModRouter.ProxyConfig()
	ModRouter.AddRoute(path, moduleProxyHandler(scheme, transport, *modApi.Endpoint))

	return nil
}

// moduleProxyTransport returns the round tripper used to reach module APIs.
// When auth is enabled it presents the control plane client certificate.
func moduleProxyTransport(authEnabled bool) (http.RoundTripper, error) {
	// return the default transport for plain http
	if !authEnabled {
		return http.DefaultTransport, nil
	}

	configDir := "/etc/threeport"

	// load the control plane client certificate and private key
	cert, err := tls.LoadX509KeyPair(
		filepath.Join(configDir, "cert/tls.crt"),
		filepath.Join(configDir, "cert/tls.key"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load control plane client certificate: %w", err)
	}

	// load the certificate authority used to verify the module API server
	caCert, err := os.ReadFile(filepath.Join(configDir, "ca/tls.crt"))
	if err != nil {
		return nil, fmt.Errorf("failed to load certificate authority: %w", err)
	}
	// add the CA to a new certificate pool
	caCertPool := x509.NewCertPool()
	if ok := caCertPool.AppendCertsFromPEM(caCert); !ok {
		return nil, fmt.Errorf("failed to parse certificate authority")
	}

	// return a transport that presents the client certificate
	return &http.Transport{
		TLSClientConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
			RootCAs:      caCertPool,
		},
	}, nil
}

// SetProxyConfig sets the scheme and transport used to reach module APIs.
func (e *ModuleRouter) SetProxyConfig(scheme string, transport http.RoundTripper) {
	e.proxyScheme = scheme
	e.proxyTransport = transport
}

// ProxyConfig returns the scheme and transport used to reach module APIs.
func (e *ModuleRouter) ProxyConfig() (string, http.RoundTripper) {
	return e.proxyScheme, e.proxyTransport
}

// AddRoute adds a new route to the dynamic route map
func (e *ModuleRouter) AddRoute(path string, handler echo.HandlerFunc) {
	e.routes.Store(path, handler)
}

// RemoveRoute removes a route from the dynamic route map
func (e *ModuleRouter) RemoveRoute(path string) {
	e.routes.Delete(path)
}

// ServeModuleRoutes checks if a dynamic route exists.  If it does, it
// returns the handler function for that route.  If not, it pases it on to the
// next handler func to continue normal request processing.
func (e *ModuleRouter) ServeModuleRoutes(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		requestPath := c.Request().URL.Path

		var matchedHandler echo.HandlerFunc
		e.routes.Range(func(route, handler interface{}) bool {
			if matchRoute(route.(string), requestPath) {
				matchedHandler = handler.(echo.HandlerFunc)
				return false // stop iterating if we find a match
			}
			return true // continue iteration
		})

		if matchedHandler != nil {
			return matchedHandler(c)
		}

		return next(c)
	}
}

// matchRoute matches a registered route path to the path from an API request.
// If a registered path matches the beginning of a request path it returns true
// as a match and ignores anything else on the request path, such as an object
// ID.
func matchRoute(registeredPath, requestedPath string) bool {
	registeredPathParsed := strings.Split(registeredPath, "/")
	requestedPathParsed := strings.Split(requestedPath, "/")

	elementCount := 0
	for elementCount < len(registeredPathParsed) {
		if registeredPathParsed[elementCount] != requestedPathParsed[elementCount] {
			return false
		}
		elementCount++
	}

	return true
}
