// generated by 'threeport-sdk gen' - do not edit

package routes

import (
	echo "github.com/labstack/echo/v4"
	handlers "github.com/threeport/threeport/pkg/api-server/v0/handlers"
	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// LogBackendRoutes sets up all routes for the LogBackend handlers.
func LogBackendRoutes(e *echo.Echo, h *handlers.Handler) {
	e.GET("/log-backends/versions", h.GetLogBackendVersions)

	e.POST(v0.PathLogBackends, h.AddLogBackend)
	e.GET(v0.PathLogBackends, h.GetLogBackends)
	e.GET(v0.PathLogBackends+"/:id", h.GetLogBackend)
	e.PATCH(v0.PathLogBackends+"/:id", h.UpdateLogBackend)
	e.PUT(v0.PathLogBackends+"/:id", h.ReplaceLogBackend)
	e.DELETE(v0.PathLogBackends+"/:id", h.DeleteLogBackend)
}

// LogStorageDefinitionRoutes sets up all routes for the LogStorageDefinition handlers.
func LogStorageDefinitionRoutes(e *echo.Echo, h *handlers.Handler) {
	e.GET("/log-storage-definitions/versions", h.GetLogStorageDefinitionVersions)

	e.POST(v0.PathLogStorageDefinitions, h.AddLogStorageDefinition)
	e.GET(v0.PathLogStorageDefinitions, h.GetLogStorageDefinitions)
	e.GET(v0.PathLogStorageDefinitions+"/:id", h.GetLogStorageDefinition)
	e.PATCH(v0.PathLogStorageDefinitions+"/:id", h.UpdateLogStorageDefinition)
	e.PUT(v0.PathLogStorageDefinitions+"/:id", h.ReplaceLogStorageDefinition)
	e.DELETE(v0.PathLogStorageDefinitions+"/:id", h.DeleteLogStorageDefinition)
}

// LogStorageInstanceRoutes sets up all routes for the LogStorageInstance handlers.
func LogStorageInstanceRoutes(e *echo.Echo, h *handlers.Handler) {
	e.GET("/log-storage-instances/versions", h.GetLogStorageInstanceVersions)

	e.POST(v0.PathLogStorageInstances, h.AddLogStorageInstance)
	e.GET(v0.PathLogStorageInstances, h.GetLogStorageInstances)
	e.GET(v0.PathLogStorageInstances+"/:id", h.GetLogStorageInstance)
	e.PATCH(v0.PathLogStorageInstances+"/:id", h.UpdateLogStorageInstance)
	e.PUT(v0.PathLogStorageInstances+"/:id", h.ReplaceLogStorageInstance)
	e.DELETE(v0.PathLogStorageInstances+"/:id", h.DeleteLogStorageInstance)
}
