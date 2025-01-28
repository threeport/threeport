package v0

import (
	"fmt"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// ModuleRouter contains the route paths that are mapped to their handler
// functions.
type ModuleRouter struct {
	routes sync.Map
}

var ModRouter = ModuleRouter{
	routes: sync.Map{},
}

// InitModuleRouter initializes an module router.  It first queries the
// database for any existing module APIs and their routes.  It then adds
// those route paths so that API requests using the module object REST paths
// are proxied to the module API.  It then instructs the echo server to use
// the ServeModuleRoutes method as middleware so that module paths are
// checked first when API requests are received.
func InitModuleRouter(
	db *gorm.DB,
	e *echo.Echo,
) error {
	var moduleApis []ModuleApi
	if result := db.Preload("ModuleApiRoutes").Find(&moduleApis); result.Error != nil {
		return fmt.Errorf("failed to query module APIs from database: %w", result.Error)
	}

	for _, modApi := range moduleApis {
		for _, apiRoute := range modApi.ModuleApiRoutes {
			ModRouter.AddRoute(*apiRoute.Path, func(c echo.Context) error {
				proxyUrl, err := url.Parse(
					fmt.Sprintf("http://%s", *modApi.Endpoint),
				)
				if err != nil {
					return fmt.Errorf("failed to parse module's proxy target URL: %w", err)
				}
				proxy := httputil.NewSingleHostReverseProxy(proxyUrl)
				proxy.ServeHTTP(c.Response().Writer, c.Request())
				return nil
			})
		}
	}

	e.Use(ModRouter.ServeModuleRoutes)

	return nil
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
