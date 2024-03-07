// generated by 'threeport-sdk codegen api-version' - do not edit

package v0

import (
	"fmt"
	"net/http"
)

// DeleteObjectByTypeAndID deletes an instance given a string representation of its type and ID.
func DeleteObjectByTypeAndID(apiClient *http.Client, apiAddr string, objectType string, id uint) error {

	switch objectType {
	case "v0.Profile":
		if _, err := DeleteProfile(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete Profile: %w", err)
		}
	case "v0.Tier":
		if _, err := DeleteTier(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete Tier: %w", err)
		}
	case "v0.AttachedObjectReference":
		if _, err := DeleteAttachedObjectReference(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete AttachedObjectReference: %w", err)
		}
	case "v0.AwsAccount":
		if _, err := DeleteAwsAccount(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete AwsAccount: %w", err)
		}
	case "v0.AwsEksKubernetesRuntimeDefinition":
		if _, err := DeleteAwsEksKubernetesRuntimeDefinition(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete AwsEksKubernetesRuntimeDefinition: %w", err)
		}
	case "v0.AwsEksKubernetesRuntimeInstance":
		if _, err := DeleteAwsEksKubernetesRuntimeInstance(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete AwsEksKubernetesRuntimeInstance: %w", err)
		}
	case "v0.AwsRelationalDatabaseDefinition":
		if _, err := DeleteAwsRelationalDatabaseDefinition(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete AwsRelationalDatabaseDefinition: %w", err)
		}
	case "v0.AwsRelationalDatabaseInstance":
		if _, err := DeleteAwsRelationalDatabaseInstance(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete AwsRelationalDatabaseInstance: %w", err)
		}
	case "v0.AwsObjectStorageBucketDefinition":
		if _, err := DeleteAwsObjectStorageBucketDefinition(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete AwsObjectStorageBucketDefinition: %w", err)
		}
	case "v0.AwsObjectStorageBucketInstance":
		if _, err := DeleteAwsObjectStorageBucketInstance(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete AwsObjectStorageBucketInstance: %w", err)
		}
	case "v0.ControlPlaneDefinition":
		if _, err := DeleteControlPlaneDefinition(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete ControlPlaneDefinition: %w", err)
		}
	case "v0.ControlPlaneInstance":
		if _, err := DeleteControlPlaneInstance(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete ControlPlaneInstance: %w", err)
		}
	case "v0.GatewayDefinition":
		if _, err := DeleteGatewayDefinition(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete GatewayDefinition: %w", err)
		}
	case "v0.GatewayInstance":
		if _, err := DeleteGatewayInstance(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete GatewayInstance: %w", err)
		}
	case "v0.GatewayHttpPort":
		if _, err := DeleteGatewayHttpPort(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete GatewayHttpPort: %w", err)
		}
	case "v0.GatewayTcpPort":
		if _, err := DeleteGatewayTcpPort(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete GatewayTcpPort: %w", err)
		}
	case "v0.DomainNameDefinition":
		if _, err := DeleteDomainNameDefinition(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete DomainNameDefinition: %w", err)
		}
	case "v0.DomainNameInstance":
		if _, err := DeleteDomainNameInstance(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete DomainNameInstance: %w", err)
		}
	case "v0.HelmWorkloadDefinition":
		if _, err := DeleteHelmWorkloadDefinition(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete HelmWorkloadDefinition: %w", err)
		}
	case "v0.HelmWorkloadInstance":
		if _, err := DeleteHelmWorkloadInstance(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete HelmWorkloadInstance: %w", err)
		}
	case "v0.KubernetesRuntimeDefinition":
		if _, err := DeleteKubernetesRuntimeDefinition(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete KubernetesRuntimeDefinition: %w", err)
		}
	case "v0.KubernetesRuntimeInstance":
		if _, err := DeleteKubernetesRuntimeInstance(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete KubernetesRuntimeInstance: %w", err)
		}
	case "v0.LogBackend":
		if _, err := DeleteLogBackend(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete LogBackend: %w", err)
		}
	case "v0.LogStorageDefinition":
		if _, err := DeleteLogStorageDefinition(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete LogStorageDefinition: %w", err)
		}
	case "v0.LogStorageInstance":
		if _, err := DeleteLogStorageInstance(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete LogStorageInstance: %w", err)
		}
	case "v0.ObservabilityStackDefinition":
		if _, err := DeleteObservabilityStackDefinition(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete ObservabilityStackDefinition: %w", err)
		}
	case "v0.ObservabilityStackInstance":
		if _, err := DeleteObservabilityStackInstance(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete ObservabilityStackInstance: %w", err)
		}
	case "v0.ObservabilityDashboardDefinition":
		if _, err := DeleteObservabilityDashboardDefinition(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete ObservabilityDashboardDefinition: %w", err)
		}
	case "v0.ObservabilityDashboardInstance":
		if _, err := DeleteObservabilityDashboardInstance(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete ObservabilityDashboardInstance: %w", err)
		}
	case "v0.MetricsDefinition":
		if _, err := DeleteMetricsDefinition(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete MetricsDefinition: %w", err)
		}
	case "v0.MetricsInstance":
		if _, err := DeleteMetricsInstance(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete MetricsInstance: %w", err)
		}
	case "v0.LoggingDefinition":
		if _, err := DeleteLoggingDefinition(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete LoggingDefinition: %w", err)
		}
	case "v0.LoggingInstance":
		if _, err := DeleteLoggingInstance(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete LoggingInstance: %w", err)
		}
	case "v0.SecretDefinition":
		if _, err := DeleteSecretDefinition(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete SecretDefinition: %w", err)
		}
	case "v0.SecretInstance":
		if _, err := DeleteSecretInstance(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete SecretInstance: %w", err)
		}
	case "v0.TerraformDefinition":
		if _, err := DeleteTerraformDefinition(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete TerraformDefinition: %w", err)
		}
	case "v0.TerraformInstance":
		if _, err := DeleteTerraformInstance(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete TerraformInstance: %w", err)
		}
	case "v0.WorkloadDefinition":
		if _, err := DeleteWorkloadDefinition(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete WorkloadDefinition: %w", err)
		}
	case "v0.WorkloadResourceDefinition":
		if _, err := DeleteWorkloadResourceDefinition(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete WorkloadResourceDefinition: %w", err)
		}
	case "v0.WorkloadInstance":
		if _, err := DeleteWorkloadInstance(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete WorkloadInstance: %w", err)
		}
	case "v0.WorkloadResourceInstance":
		if _, err := DeleteWorkloadResourceInstance(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete WorkloadResourceInstance: %w", err)
		}
	case "v0.WorkloadEvent":
		if _, err := DeleteWorkloadEvent(apiClient, apiAddr, id); err != nil {
			return fmt.Errorf("failed to delete WorkloadEvent: %w", err)
		}

	}

	return nil
}
