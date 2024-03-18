// generated by 'threeport-sdk codegen api-model' - do not edit

package routes

import (
	echo "github.com/labstack/echo/v4"
	handlers "github.com/threeport/threeport/pkg/api-server/v1/handlers"
	v1 "github.com/threeport/threeport/pkg/api/v1"
)

// WorkloadInstanceRoutes sets up all routes for the WorkloadInstance handlers.
func WorkloadInstanceRoutes(e *echo.Echo, h *handlers.Handler) {
	e.GET("/workload-instances/versions", h.GetWorkloadInstanceVersions)

	e.POST(v1.PathWorkloadInstances, h.AddWorkloadInstance)
	e.GET(v1.PathWorkloadInstances, h.GetWorkloadInstances)
	e.GET(v1.PathWorkloadInstances+"/:id", h.GetWorkloadInstance)
	e.PATCH(v1.PathWorkloadInstances+"/:id", h.UpdateWorkloadInstance)
	e.PUT(v1.PathWorkloadInstances+"/:id", h.ReplaceWorkloadInstance)
	e.DELETE(v1.PathWorkloadInstances+"/:id", h.DeleteWorkloadInstance)
}
