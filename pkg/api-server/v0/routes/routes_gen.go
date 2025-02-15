// generated by 'threeport-sdk gen' - do not edit

package routes

import (
	echo "github.com/labstack/echo/v4"
	handlers "github.com/threeport/threeport/pkg/api-server/v0/handlers"
)

// AddRoutes adds routes for all objects of a particular API version.
func AddRoutes(e *echo.Echo, h *handlers.Handler) {
	AttachedObjectReferenceRoutes(e, h)
	AwsAccountRoutes(e, h)
	AwsEksKubernetesRuntimeDefinitionRoutes(e, h)
	AwsEksKubernetesRuntimeInstanceRoutes(e, h)
	AwsObjectStorageBucketDefinitionRoutes(e, h)
	AwsObjectStorageBucketInstanceRoutes(e, h)
	AwsRelationalDatabaseDefinitionRoutes(e, h)
	AwsRelationalDatabaseInstanceRoutes(e, h)
	ControlPlaneDefinitionRoutes(e, h)
	ControlPlaneInstanceRoutes(e, h)
	DomainNameDefinitionRoutes(e, h)
	DomainNameInstanceRoutes(e, h)
	EventRoutes(e, h)
	GatewayDefinitionRoutes(e, h)
	GatewayHttpPortRoutes(e, h)
	GatewayInstanceRoutes(e, h)
	GatewayTcpPortRoutes(e, h)
	HelmWorkloadDefinitionRoutes(e, h)
	HelmWorkloadInstanceRoutes(e, h)
	KubernetesRuntimeDefinitionRoutes(e, h)
	KubernetesRuntimeInstanceRoutes(e, h)
	LogBackendRoutes(e, h)
	LogStorageDefinitionRoutes(e, h)
	LogStorageInstanceRoutes(e, h)
	LoggingDefinitionRoutes(e, h)
	LoggingInstanceRoutes(e, h)
	MetricsDefinitionRoutes(e, h)
	MetricsInstanceRoutes(e, h)
	ModuleApiRoutes(e, h)
	ModuleApiRouteRoutes(e, h)
	ObservabilityDashboardDefinitionRoutes(e, h)
	ObservabilityDashboardInstanceRoutes(e, h)
	ObservabilityStackDefinitionRoutes(e, h)
	ObservabilityStackInstanceRoutes(e, h)
	ProfileRoutes(e, h)
	SecretDefinitionRoutes(e, h)
	SecretInstanceRoutes(e, h)
	TerraformDefinitionRoutes(e, h)
	TerraformInstanceRoutes(e, h)
	TierRoutes(e, h)
	WorkloadDefinitionRoutes(e, h)
	WorkloadEventRoutes(e, h)
	WorkloadInstanceRoutes(e, h)
	WorkloadResourceDefinitionRoutes(e, h)
	WorkloadResourceInstanceRoutes(e, h)
}
