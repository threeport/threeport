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

// ModuleRouter maps module route paths to their handler functions and holds the
// settings the module proxy uses to reach module API servers. The scheme and
// transport are written once before the server accepts a request and read later
// when a module registers a route; a second writer would need a lock.
type ModuleRouter struct {
	routes         sync.Map
	proxyScheme    string
	proxyTransport http.RoundTripper
}

var ModRouter = ModuleRouter{
	routes:         sync.Map{},
	proxyScheme:    "http",
	proxyTransport: http.DefaultTransport,
}

// InitModuleRouter initializes an module router.  It first queries the
// database for any existing module APIs and their routes.  It then adds
// those route paths so that API requests using the module object REST paths
// are proxied to the module API.  It then instructs the echo server to use
// the ServeModuleRoutes method as middleware so that module paths are
// checked first when API requests are received.  When authEnabled is true the
// proxy reaches each module over https and presents the control plane client
// certificate so the module's caller-capture middleware reads the control
// plane organizational unit; when false it proxies over plain http.
func InitModuleRouter(
	db *gorm.DB,
	e *echo.Echo,
	authEnabled bool,
) error {
	var moduleApis []ModuleApi
	if result := db.Preload("ModuleApiRoutes").Where("core = ?", false).Find(&moduleApis); result.Error != nil {
		return fmt.Errorf("failed to query module APIs from database: %w", result.Error)
	}

	// build the transport that presents the control plane client certificate
	// and trusts the control plane certificate authority so module proxy
	// requests authenticate as the control plane over https
	transport, err := moduleProxyTransport(authEnabled)
	if err != nil {
		return fmt.Errorf("failed to build module proxy transport: %w", err)
	}

	scheme := "http"
	if authEnabled {
		scheme = "https"
	}
	// record the scheme and transport on the router so a module that registers
	// a route after startup builds its proxy the same way the routes below do
	ModRouter.SetProxyConfig(scheme, transport)

	for _, modApi := range moduleApis {
		for _, apiRoute := range modApi.ModuleApiRoutes {
			ModRouter.AddRoute(*apiRoute.Path, func(c echo.Context) error {
				proxyUrl, err := url.Parse(
					fmt.Sprintf("%s://%s", scheme, *modApi.Endpoint),
				)
				if err != nil {
					return fmt.Errorf("failed to parse module's proxy target URL: %w", err)
				}
				proxy := httputil.NewSingleHostReverseProxy(proxyUrl)
				proxy.Transport = transport
				proxy.ServeHTTP(c.Response().Writer, c.Request())
				return nil
			})
		}
	}

	e.Use(ModRouter.ServeModuleRoutes)

	return nil
}

// moduleProxyTransport returns the http transport the module proxy uses to
// reach module API servers.  When authEnabled is false it returns the default
// transport for plain http.  When true it loads the control plane client
// certificate and certificate authority from the mounted secret and returns a
// transport that presents the client certificate over https so the module API
// server reads the control plane organizational unit from the peer certificate.
func moduleProxyTransport(authEnabled bool) (http.RoundTripper, error) {
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
	caCertPool := x509.NewCertPool()
	if ok := caCertPool.AppendCertsFromPEM(caCert); !ok {
		return nil, fmt.Errorf("failed to parse certificate authority")
	}

	return &http.Transport{
		TLSClientConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
			RootCAs:      caCertPool,
		},
	}, nil
}

// SetProxyConfig sets the scheme and transport the module proxy uses to reach
// module API servers. Call it once at startup so a later-registered route
// reaches its module the same way the routes present at startup do.
func (e *ModuleRouter) SetProxyConfig(scheme string, transport http.RoundTripper) {
	e.proxyScheme = scheme
	e.proxyTransport = transport
}

// ProxyConfig returns the scheme and transport the module proxy uses to reach
// module API servers.  It returns both together so a caller building a proxy
// cannot pair an https scheme with a plain http transport.
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
