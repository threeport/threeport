// generated by 'threeport-codegen api-model' - do not edit

package routes

import (
	echo "github.com/labstack/echo/v4"
	handlers "github.com/threeport/threeport/internal/api/handlers"
	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// GatewayDefinitionRoutes sets up all routes for the GatewayDefinition handlers.
func GatewayDefinitionRoutes(e *echo.Echo, h *handlers.Handler) {
	e.GET("/gateway-definitions/versions", h.GetGatewayDefinitionVersions)

	e.POST(v0.PathGatewayDefinitions, h.AddGatewayDefinition)
	e.GET(v0.PathGatewayDefinitions, h.GetGatewayDefinitions)
	e.GET(v0.PathGatewayDefinitions+"/:id", h.GetGatewayDefinition)
	e.PATCH(v0.PathGatewayDefinitions+"/:id", h.UpdateGatewayDefinition)
	e.PUT(v0.PathGatewayDefinitions+"/:id", h.ReplaceGatewayDefinition)
	e.DELETE(v0.PathGatewayDefinitions+"/:id", h.DeleteGatewayDefinition)
}

// GatewayInstanceRoutes sets up all routes for the GatewayInstance handlers.
func GatewayInstanceRoutes(e *echo.Echo, h *handlers.Handler) {
	e.GET("/gateway-instances/versions", h.GetGatewayInstanceVersions)

	e.POST(v0.PathGatewayInstances, h.AddGatewayInstance)
	e.GET(v0.PathGatewayInstances, h.GetGatewayInstances)
	e.GET(v0.PathGatewayInstances+"/:id", h.GetGatewayInstance)
	e.PATCH(v0.PathGatewayInstances+"/:id", h.UpdateGatewayInstance)
	e.PUT(v0.PathGatewayInstances+"/:id", h.ReplaceGatewayInstance)
	e.DELETE(v0.PathGatewayInstances+"/:id", h.DeleteGatewayInstance)
}
