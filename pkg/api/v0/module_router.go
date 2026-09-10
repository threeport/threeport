package v0

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
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
	// query non-core module APIs; core routes are served by this process
	var moduleApis []ModuleApi
	if result := db.Preload("ModuleApiRoutes").Where("core = ?", false).Find(&moduleApis); result.Error != nil {
		return fmt.Errorf("failed to query module APIs from database: %w", result.Error)
	}

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

	// store scheme and transport for later route registration
	ModRouter.SetProxyConfig(scheme, transport)

	// add a reverse proxy handler for each module route
	for _, modApi := range moduleApis {
		for _, apiRoute := range modApi.ModuleApiRoutes {
			ModRouter.AddRoute(*apiRoute.Path, func(c echo.Context) error {
				// parse the module proxy target URL
				proxyUrl, err := url.Parse(
					fmt.Sprintf("%s://%s", scheme, *modApi.Endpoint),
				)
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
			})
		}
	}

	// register as middleware wrapping the matched or not-found handler
	e.Use(ModRouter.ServeModuleRoutes)

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

// Route returns the handler registered for path, and whether one was
// registered at all.
//
// It exists so a caller that is about to run a persist hook inside a
// transaction can capture the router's state first and put it back if the
// transaction does not commit. Path carries no unique index, so two routes may
// share one router key; a caller that removed the key outright on failure
// would take a sibling's live route down with it.
func (e *ModuleRouter) Route(path string) (echo.HandlerFunc, bool) {
	handler, ok := e.routes.Load(path)
	if !ok {
		return nil, false
	}

	routeHandler, ok := handler.(echo.HandlerFunc)

	return routeHandler, ok
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
