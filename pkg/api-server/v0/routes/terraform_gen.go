// generated by 'threeport-codegen api-model' - do not edit

package routes

import (
	echo "github.com/labstack/echo/v4"
	handlers "github.com/threeport/threeport/pkg/api-server/v0/handlers"
	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// TerraformDefinitionRoutes sets up all routes for the TerraformDefinition handlers.
func TerraformDefinitionRoutes(e *echo.Echo, h *handlers.Handler) {
	e.GET("/terraform-definitions/versions", h.GetTerraformDefinitionVersions)

	e.POST(v0.PathTerraformDefinitions, h.AddTerraformDefinition)
	e.GET(v0.PathTerraformDefinitions, h.GetTerraformDefinitions)
	e.GET(v0.PathTerraformDefinitions+"/:id", h.GetTerraformDefinition)
	e.PATCH(v0.PathTerraformDefinitions+"/:id", h.UpdateTerraformDefinition)
	e.PUT(v0.PathTerraformDefinitions+"/:id", h.ReplaceTerraformDefinition)
	e.DELETE(v0.PathTerraformDefinitions+"/:id", h.DeleteTerraformDefinition)
}

// TerraformInstanceRoutes sets up all routes for the TerraformInstance handlers.
func TerraformInstanceRoutes(e *echo.Echo, h *handlers.Handler) {
	e.GET("/terraform-instances/versions", h.GetTerraformInstanceVersions)

	e.POST(v0.PathTerraformInstances, h.AddTerraformInstance)
	e.GET(v0.PathTerraformInstances, h.GetTerraformInstances)
	e.GET(v0.PathTerraformInstances+"/:id", h.GetTerraformInstance)
	e.PATCH(v0.PathTerraformInstances+"/:id", h.UpdateTerraformInstance)
	e.PUT(v0.PathTerraformInstances+"/:id", h.ReplaceTerraformInstance)
	e.DELETE(v0.PathTerraformInstances+"/:id", h.DeleteTerraformInstance)
}
