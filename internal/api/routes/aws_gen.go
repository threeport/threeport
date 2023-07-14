// generated by 'threeport-codegen api-model' - do not edit

package routes

import (
	echo "github.com/labstack/echo/v4"
	handlers "github.com/threeport/threeport/internal/api/handlers"
	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// AwsAccountRoutes sets up all routes for the AwsAccount handlers.
func AwsAccountRoutes(e *echo.Echo, h *handlers.Handler) {
	e.GET("/aws-accounts/versions", h.GetAwsAccountVersions)

	e.POST(v0.PathAwsAccounts, h.AddAwsAccount)
	e.GET(v0.PathAwsAccounts, h.GetAwsAccounts)
	e.GET(v0.PathAwsAccounts+"/:id", h.GetAwsAccount)
	e.PATCH(v0.PathAwsAccounts+"/:id", h.UpdateAwsAccount)
	e.PUT(v0.PathAwsAccounts+"/:id", h.ReplaceAwsAccount)
	e.DELETE(v0.PathAwsAccounts+"/:id", h.DeleteAwsAccount)
}

// AwsEksKubernetesRuntimeDefinitionRoutes sets up all routes for the AwsEksKubernetesRuntimeDefinition handlers.
func AwsEksKubernetesRuntimeDefinitionRoutes(e *echo.Echo, h *handlers.Handler) {
	e.GET("/aws-eks-kubernetes-runtime-definitions/versions", h.GetAwsEksKubernetesRuntimeDefinitionVersions)

	e.POST(v0.PathAwsEksKubernetesRuntimeDefinitions, h.AddAwsEksKubernetesRuntimeDefinition)
	e.GET(v0.PathAwsEksKubernetesRuntimeDefinitions, h.GetAwsEksKubernetesRuntimeDefinitions)
	e.GET(v0.PathAwsEksKubernetesRuntimeDefinitions+"/:id", h.GetAwsEksKubernetesRuntimeDefinition)
	e.PATCH(v0.PathAwsEksKubernetesRuntimeDefinitions+"/:id", h.UpdateAwsEksKubernetesRuntimeDefinition)
	e.PUT(v0.PathAwsEksKubernetesRuntimeDefinitions+"/:id", h.ReplaceAwsEksKubernetesRuntimeDefinition)
	e.DELETE(v0.PathAwsEksKubernetesRuntimeDefinitions+"/:id", h.DeleteAwsEksKubernetesRuntimeDefinition)
}

// AwsEksKubernetesRuntimeInstanceRoutes sets up all routes for the AwsEksKubernetesRuntimeInstance handlers.
func AwsEksKubernetesRuntimeInstanceRoutes(e *echo.Echo, h *handlers.Handler) {
	e.GET("/aws-eks-kubernetes-runtime-instances/versions", h.GetAwsEksKubernetesRuntimeInstanceVersions)

	e.POST(v0.PathAwsEksKubernetesRuntimeInstances, h.AddAwsEksKubernetesRuntimeInstance)
	e.GET(v0.PathAwsEksKubernetesRuntimeInstances, h.GetAwsEksKubernetesRuntimeInstances)
	e.GET(v0.PathAwsEksKubernetesRuntimeInstances+"/:id", h.GetAwsEksKubernetesRuntimeInstance)
	e.PATCH(v0.PathAwsEksKubernetesRuntimeInstances+"/:id", h.UpdateAwsEksKubernetesRuntimeInstance)
	e.PUT(v0.PathAwsEksKubernetesRuntimeInstances+"/:id", h.ReplaceAwsEksKubernetesRuntimeInstance)
	e.DELETE(v0.PathAwsEksKubernetesRuntimeInstances+"/:id", h.DeleteAwsEksKubernetesRuntimeInstance)
}

// AwsRelationalDatabaseDefinitionRoutes sets up all routes for the AwsRelationalDatabaseDefinition handlers.
func AwsRelationalDatabaseDefinitionRoutes(e *echo.Echo, h *handlers.Handler) {
	e.GET("/aws-relational-database-definitions/versions", h.GetAwsRelationalDatabaseDefinitionVersions)

	e.POST(v0.PathAwsRelationalDatabaseDefinitions, h.AddAwsRelationalDatabaseDefinition)
	e.GET(v0.PathAwsRelationalDatabaseDefinitions, h.GetAwsRelationalDatabaseDefinitions)
	e.GET(v0.PathAwsRelationalDatabaseDefinitions+"/:id", h.GetAwsRelationalDatabaseDefinition)
	e.PATCH(v0.PathAwsRelationalDatabaseDefinitions+"/:id", h.UpdateAwsRelationalDatabaseDefinition)
	e.PUT(v0.PathAwsRelationalDatabaseDefinitions+"/:id", h.ReplaceAwsRelationalDatabaseDefinition)
	e.DELETE(v0.PathAwsRelationalDatabaseDefinitions+"/:id", h.DeleteAwsRelationalDatabaseDefinition)
}

// AwsRelationalDatabaseInstanceRoutes sets up all routes for the AwsRelationalDatabaseInstance handlers.
func AwsRelationalDatabaseInstanceRoutes(e *echo.Echo, h *handlers.Handler) {
	e.GET("/aws-relational-database-instances/versions", h.GetAwsRelationalDatabaseInstanceVersions)

	e.POST(v0.PathAwsRelationalDatabaseInstances, h.AddAwsRelationalDatabaseInstance)
	e.GET(v0.PathAwsRelationalDatabaseInstances, h.GetAwsRelationalDatabaseInstances)
	e.GET(v0.PathAwsRelationalDatabaseInstances+"/:id", h.GetAwsRelationalDatabaseInstance)
	e.PATCH(v0.PathAwsRelationalDatabaseInstances+"/:id", h.UpdateAwsRelationalDatabaseInstance)
	e.PUT(v0.PathAwsRelationalDatabaseInstances+"/:id", h.ReplaceAwsRelationalDatabaseInstance)
	e.DELETE(v0.PathAwsRelationalDatabaseInstances+"/:id", h.DeleteAwsRelationalDatabaseInstance)
}
