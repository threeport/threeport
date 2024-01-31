// generated by 'threeport-codegen api-version' - do not edit

package routes

import (
	echo "github.com/labstack/echo/v4"
	handlers "github.com/threeport/threeport/pkg/api-server/v0/handlers"
)

func AddRoutes(e *echo.Echo, h *handlers.Handler) {
	ProfileRoutes(e, h)
	TierRoutes(e, h)
	AwsAccountRoutes(e, h)
	AwsEksKubernetesRuntimeDefinitionRoutes(e, h)
	AwsEksKubernetesRuntimeInstanceRoutes(e, h)
	AwsRelationalDatabaseDefinitionRoutes(e, h)
	AwsRelationalDatabaseInstanceRoutes(e, h)
	AwsObjectStorageBucketDefinitionRoutes(e, h)
	AwsObjectStorageBucketInstanceRoutes(e, h)
	ControlPlaneDefinitionRoutes(e, h)
	ControlPlaneInstanceRoutes(e, h)
	ForwardProxyDefinitionRoutes(e, h)
	ForwardProxyInstanceRoutes(e, h)
	GatewayDefinitionRoutes(e, h)
	GatewayInstanceRoutes(e, h)
	GatewayHttpPortRoutes(e, h)
	GatewayTcpPortRoutes(e, h)
	DomainNameDefinitionRoutes(e, h)
	DomainNameInstanceRoutes(e, h)
	HelmWorkloadDefinitionRoutes(e, h)
	HelmWorkloadInstanceRoutes(e, h)
	KubernetesRuntimeDefinitionRoutes(e, h)
	KubernetesRuntimeInstanceRoutes(e, h)
	LogBackendRoutes(e, h)
	LogStorageDefinitionRoutes(e, h)
	LogStorageInstanceRoutes(e, h)
	MetricsDefinitionRoutes(e, h)
	MetricsInstanceRoutes(e, h)
	WorkloadDefinitionRoutes(e, h)
	WorkloadResourceDefinitionRoutes(e, h)
	WorkloadInstanceRoutes(e, h)
	AttachedObjectReferenceRoutes(e, h)
	WorkloadResourceInstanceRoutes(e, h)
	WorkloadEventRoutes(e, h)

}
