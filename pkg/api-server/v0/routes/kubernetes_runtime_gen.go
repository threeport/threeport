// generated by 'threeport-sdk gen' for API routes boilerplate' - do not edit

package routes

import (
	echo "github.com/labstack/echo/v4"
	handlers "github.com/threeport/threeport/pkg/api-server/v0/handlers"
	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// KubernetesRuntimeDefinitionRoutes sets up all routes for the KubernetesRuntimeDefinition handlers.
func KubernetesRuntimeDefinitionRoutes(e *echo.Echo, h *handlers.Handler) {
	e.GET("/kubernetes-runtime-definitions/versions", h.GetKubernetesRuntimeDefinitionVersions)

	e.POST(v0.PathKubernetesRuntimeDefinitions, h.AddKubernetesRuntimeDefinition)
	e.GET(v0.PathKubernetesRuntimeDefinitions, h.GetKubernetesRuntimeDefinitions)
	e.GET(v0.PathKubernetesRuntimeDefinitions+"/:id", h.GetKubernetesRuntimeDefinition)
	e.PATCH(v0.PathKubernetesRuntimeDefinitions+"/:id", h.UpdateKubernetesRuntimeDefinition)
	e.PUT(v0.PathKubernetesRuntimeDefinitions+"/:id", h.ReplaceKubernetesRuntimeDefinition)
	e.DELETE(v0.PathKubernetesRuntimeDefinitions+"/:id", h.DeleteKubernetesRuntimeDefinition)
}

// KubernetesRuntimeInstanceRoutes sets up all routes for the KubernetesRuntimeInstance handlers.
func KubernetesRuntimeInstanceRoutes(e *echo.Echo, h *handlers.Handler) {
	e.GET("/kubernetes-runtime-instances/versions", h.GetKubernetesRuntimeInstanceVersions)

	e.POST(v0.PathKubernetesRuntimeInstances, h.AddKubernetesRuntimeInstance)
	e.GET(v0.PathKubernetesRuntimeInstances, h.GetKubernetesRuntimeInstances)
	e.GET(v0.PathKubernetesRuntimeInstances+"/:id", h.GetKubernetesRuntimeInstance)
	e.PATCH(v0.PathKubernetesRuntimeInstances+"/:id", h.UpdateKubernetesRuntimeInstance)
	e.PUT(v0.PathKubernetesRuntimeInstances+"/:id", h.ReplaceKubernetesRuntimeInstance)
	e.DELETE(v0.PathKubernetesRuntimeInstances+"/:id", h.DeleteKubernetesRuntimeInstance)
}
