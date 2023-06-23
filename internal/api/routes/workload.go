package routes

import (
	"github.com/labstack/echo/v4"

	"github.com/threeport/threeport/internal/api/handlers"
	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// WorkloadResourceDefinitionSetRoutes sets up all routes for the
// WorkloadResourceDefinition set handlers.  This custom API route operates on
// batches of workload resource definitions for efficiency.
func WorkloadResourceDefinitionSetRoutes(e *echo.Echo, h *handlers.Handler) {
	// TODO: Version collection needs to be unravelled from tagged fields and
	// refactored to be sane and extensible.  Currently there's not good way to
	// manage versions for custom REST endpoints like this.
	//e.GET("/workload_resource_definition_sets/versions", h.GetWorkloadResourceDefinitionVersions)

	e.POST(v0.PathWorkloadResourceDefinitionSets, h.AddWorkloadResourceDefinitions)
}

// WorkloadEventSetRoutes sets up all routes for the WorkloadEvent set handlers.
// This custom API route operates on batches of workload events for efficiency.
func WorkloadEventSetRoutes(e *echo.Echo, h *handlers.Handler) {
	e.GET(v0.PathWorkloadEventSets+"/:workloadInstanceID", h.GetWorkloadEventSet)
	e.DELETE(v0.PathWorkloadEventSets+"/:workloadInstanceID", h.DeleteWorkloadEventSet)
}
