// generated by 'threeport-sdk gen' - do not edit

package routes

import (
	echo "github.com/labstack/echo/v4"
	handlers "github.com/threeport/threeport/pkg/api-server/v0/handlers"
	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// LoggingDefinitionRoutes sets up all routes for the LoggingDefinition handlers.
func LoggingDefinitionRoutes(e *echo.Echo, h *handlers.Handler) {
	e.GET(v0.PathLoggingDefinitionVersions, h.GetLoggingDefinitionVersions)

	e.POST(v0.PathLoggingDefinitions, h.AddLoggingDefinition)
	e.GET(v0.PathLoggingDefinitions, h.GetLoggingDefinitions)
	e.GET(v0.PathLoggingDefinitions+"/:id", h.GetLoggingDefinition)
	e.PATCH(v0.PathLoggingDefinitions+"/:id", h.UpdateLoggingDefinition)
	e.PUT(v0.PathLoggingDefinitions+"/:id", h.ReplaceLoggingDefinition)
	e.DELETE(v0.PathLoggingDefinitions+"/:id", h.DeleteLoggingDefinition)
}

// LoggingInstanceRoutes sets up all routes for the LoggingInstance handlers.
func LoggingInstanceRoutes(e *echo.Echo, h *handlers.Handler) {
	e.GET(v0.PathLoggingInstanceVersions, h.GetLoggingInstanceVersions)

	e.POST(v0.PathLoggingInstances, h.AddLoggingInstance)
	e.GET(v0.PathLoggingInstances, h.GetLoggingInstances)
	e.GET(v0.PathLoggingInstances+"/:id", h.GetLoggingInstance)
	e.PATCH(v0.PathLoggingInstances+"/:id", h.UpdateLoggingInstance)
	e.PUT(v0.PathLoggingInstances+"/:id", h.ReplaceLoggingInstance)
	e.DELETE(v0.PathLoggingInstances+"/:id", h.DeleteLoggingInstance)
}

// MetricsDefinitionRoutes sets up all routes for the MetricsDefinition handlers.
func MetricsDefinitionRoutes(e *echo.Echo, h *handlers.Handler) {
	e.GET(v0.PathMetricsDefinitionVersions, h.GetMetricsDefinitionVersions)

	e.POST(v0.PathMetricsDefinitions, h.AddMetricsDefinition)
	e.GET(v0.PathMetricsDefinitions, h.GetMetricsDefinitions)
	e.GET(v0.PathMetricsDefinitions+"/:id", h.GetMetricsDefinition)
	e.PATCH(v0.PathMetricsDefinitions+"/:id", h.UpdateMetricsDefinition)
	e.PUT(v0.PathMetricsDefinitions+"/:id", h.ReplaceMetricsDefinition)
	e.DELETE(v0.PathMetricsDefinitions+"/:id", h.DeleteMetricsDefinition)
}

// MetricsInstanceRoutes sets up all routes for the MetricsInstance handlers.
func MetricsInstanceRoutes(e *echo.Echo, h *handlers.Handler) {
	e.GET(v0.PathMetricsInstanceVersions, h.GetMetricsInstanceVersions)

	e.POST(v0.PathMetricsInstances, h.AddMetricsInstance)
	e.GET(v0.PathMetricsInstances, h.GetMetricsInstances)
	e.GET(v0.PathMetricsInstances+"/:id", h.GetMetricsInstance)
	e.PATCH(v0.PathMetricsInstances+"/:id", h.UpdateMetricsInstance)
	e.PUT(v0.PathMetricsInstances+"/:id", h.ReplaceMetricsInstance)
	e.DELETE(v0.PathMetricsInstances+"/:id", h.DeleteMetricsInstance)
}

// ObservabilityDashboardDefinitionRoutes sets up all routes for the ObservabilityDashboardDefinition handlers.
func ObservabilityDashboardDefinitionRoutes(e *echo.Echo, h *handlers.Handler) {
	e.GET(v0.PathObservabilityDashboardDefinitionVersions, h.GetObservabilityDashboardDefinitionVersions)

	e.POST(v0.PathObservabilityDashboardDefinitions, h.AddObservabilityDashboardDefinition)
	e.GET(v0.PathObservabilityDashboardDefinitions, h.GetObservabilityDashboardDefinitions)
	e.GET(v0.PathObservabilityDashboardDefinitions+"/:id", h.GetObservabilityDashboardDefinition)
	e.PATCH(v0.PathObservabilityDashboardDefinitions+"/:id", h.UpdateObservabilityDashboardDefinition)
	e.PUT(v0.PathObservabilityDashboardDefinitions+"/:id", h.ReplaceObservabilityDashboardDefinition)
	e.DELETE(v0.PathObservabilityDashboardDefinitions+"/:id", h.DeleteObservabilityDashboardDefinition)
}

// ObservabilityDashboardInstanceRoutes sets up all routes for the ObservabilityDashboardInstance handlers.
func ObservabilityDashboardInstanceRoutes(e *echo.Echo, h *handlers.Handler) {
	e.GET(v0.PathObservabilityDashboardInstanceVersions, h.GetObservabilityDashboardInstanceVersions)

	e.POST(v0.PathObservabilityDashboardInstances, h.AddObservabilityDashboardInstance)
	e.GET(v0.PathObservabilityDashboardInstances, h.GetObservabilityDashboardInstances)
	e.GET(v0.PathObservabilityDashboardInstances+"/:id", h.GetObservabilityDashboardInstance)
	e.PATCH(v0.PathObservabilityDashboardInstances+"/:id", h.UpdateObservabilityDashboardInstance)
	e.PUT(v0.PathObservabilityDashboardInstances+"/:id", h.ReplaceObservabilityDashboardInstance)
	e.DELETE(v0.PathObservabilityDashboardInstances+"/:id", h.DeleteObservabilityDashboardInstance)
}

// ObservabilityStackDefinitionRoutes sets up all routes for the ObservabilityStackDefinition handlers.
func ObservabilityStackDefinitionRoutes(e *echo.Echo, h *handlers.Handler) {
	e.GET(v0.PathObservabilityStackDefinitionVersions, h.GetObservabilityStackDefinitionVersions)

	e.POST(v0.PathObservabilityStackDefinitions, h.AddObservabilityStackDefinition)
	e.GET(v0.PathObservabilityStackDefinitions, h.GetObservabilityStackDefinitions)
	e.GET(v0.PathObservabilityStackDefinitions+"/:id", h.GetObservabilityStackDefinition)
	e.PATCH(v0.PathObservabilityStackDefinitions+"/:id", h.UpdateObservabilityStackDefinition)
	e.PUT(v0.PathObservabilityStackDefinitions+"/:id", h.ReplaceObservabilityStackDefinition)
	e.DELETE(v0.PathObservabilityStackDefinitions+"/:id", h.DeleteObservabilityStackDefinition)
}

// ObservabilityStackInstanceRoutes sets up all routes for the ObservabilityStackInstance handlers.
func ObservabilityStackInstanceRoutes(e *echo.Echo, h *handlers.Handler) {
	e.GET(v0.PathObservabilityStackInstanceVersions, h.GetObservabilityStackInstanceVersions)

	e.POST(v0.PathObservabilityStackInstances, h.AddObservabilityStackInstance)
	e.GET(v0.PathObservabilityStackInstances, h.GetObservabilityStackInstances)
	e.GET(v0.PathObservabilityStackInstances+"/:id", h.GetObservabilityStackInstance)
	e.PATCH(v0.PathObservabilityStackInstances+"/:id", h.UpdateObservabilityStackInstance)
	e.PUT(v0.PathObservabilityStackInstances+"/:id", h.ReplaceObservabilityStackInstance)
	e.DELETE(v0.PathObservabilityStackInstances+"/:id", h.DeleteObservabilityStackInstance)
}
