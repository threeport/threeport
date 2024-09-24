package v0

import (
	"fmt"
	"net/http/httputil"
	"net/url"
	"sync"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// ExtensionRouter contains the route paths that are mapped to their handler
// functions.
type ExtensionRouter struct {
	routes map[string]echo.HandlerFunc
	mu     sync.RWMutex
}

var ExtRouter = ExtensionRouter{
	routes: make(map[string]echo.HandlerFunc),
}

// InitExtensionRouter initializes an extension router.  It first queries the
// database for any existing extension APIs and their routes.  It then adds
// those route paths so that API requests using the extension object REST paths
// are proxied to the extension API.  It then instructs the echo server to use
// the ServeExtensionRoutes method as middleware so that extension paths are
// checked first when API requests are received.
func InitExtensionRouter(
	db *gorm.DB,
	e *echo.Echo,
) error {
	var extensionApis []ExtensionApi
	if result := db.Preload("ExtensionApiRoutes").Find(&extensionApis); result.Error != nil {
		return fmt.Errorf("failed to query extension APIs from database: %w", result.Error)
	}

	for _, extApi := range extensionApis {
		for _, apiRoute := range extApi.ExtensionApiRoutes {
			ExtRouter.AddRoute(*apiRoute.Path, func(c echo.Context) error {
				proxyUrl, err := url.Parse(
					fmt.Sprintf("http://%s", *extApi.Endpoint),
				)
				if err != nil {
					return fmt.Errorf("failed to parse extension's proxy target URL: %w", err)
				}
				proxy := httputil.NewSingleHostReverseProxy(proxyUrl)
				proxy.ServeHTTP(c.Response().Writer, c.Request())
				return nil
			})
		}
	}

	e.Use(ExtRouter.ServeExtensionRoutes)

	return nil
}

// AddRoute safely adds a new route to the dynamic route map
func (e *ExtensionRouter) AddRoute(path string, handler echo.HandlerFunc) {
	e.mu.Lock()
	e.routes[path] = handler
	e.mu.Unlock()
}

// RemoveRoute safely removes a route from the dynamic route map
func (e *ExtensionRouter) RemoveRoute(path string) {
	e.mu.Lock()
	delete(e.routes, path)
	e.mu.Unlock()
}

// ServeExtensionRoutes checks if a dynamic route exists, and if not, it lets Echo continue processing
func (e *ExtensionRouter) ServeExtensionRoutes(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		e.mu.RLock()
		handler, exists := e.routes[c.Request().URL.Path]
		e.mu.RUnlock()

		// proxy to the extension API if found
		if exists {
			return handler(c)
		}

		// if no proxy path found, pass onto the next route handler
		return next(c)
	}
}
