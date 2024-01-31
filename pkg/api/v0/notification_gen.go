// generated by 'threeport-codegen api-version' - do not edit
// +threeport-codegen route-exclude
// +threeport-codegen database-exclude

package v0

import "errors"

// GetSubjectByReconcilerName returns the subject for a reconciler's name.
func GetSubjectByReconcilerName(name string) (string, error) {
	switch name {
	case "AwsEksKubernetesRuntimeInstanceReconciler":
		return AwsEksKubernetesRuntimeInstanceSubject, nil
	case "AwsRelationalDatabaseInstanceReconciler":
		return AwsRelationalDatabaseInstanceSubject, nil
	case "AwsObjectStorageBucketInstanceReconciler":
		return AwsObjectStorageBucketInstanceSubject, nil
	case "ControlPlaneDefinitionReconciler":
		return ControlPlaneDefinitionSubject, nil
	case "ControlPlaneInstanceReconciler":
		return ControlPlaneInstanceSubject, nil
	case "GatewayDefinitionReconciler":
		return GatewayDefinitionSubject, nil
	case "GatewayInstanceReconciler":
		return GatewayInstanceSubject, nil
	case "DomainNameInstanceReconciler":
		return DomainNameInstanceSubject, nil
	case "HelmWorkloadDefinitionReconciler":
		return HelmWorkloadDefinitionSubject, nil
	case "HelmWorkloadInstanceReconciler":
		return HelmWorkloadInstanceSubject, nil
	case "KubernetesRuntimeDefinitionReconciler":
		return KubernetesRuntimeDefinitionSubject, nil
	case "KubernetesRuntimeInstanceReconciler":
		return KubernetesRuntimeInstanceSubject, nil
	case "ObservabilityStackDefinitionReconciler":
		return ObservabilityStackDefinitionSubject, nil
	case "ObservabilityStackInstanceReconciler":
		return ObservabilityStackInstanceSubject, nil
	case "ObservabilityDashboardDefinitionReconciler":
		return ObservabilityDashboardDefinitionSubject, nil
	case "ObservabilityDashboardInstanceReconciler":
		return ObservabilityDashboardInstanceSubject, nil
	case "MetricsDefinitionReconciler":
		return MetricsDefinitionSubject, nil
	case "MetricsInstanceReconciler":
		return MetricsInstanceSubject, nil
	case "LoggingDefinitionReconciler":
		return LoggingDefinitionSubject, nil
	case "LoggingInstanceReconciler":
		return LoggingInstanceSubject, nil
	case "WorkloadDefinitionReconciler":
		return WorkloadDefinitionSubject, nil
	case "WorkloadInstanceReconciler":
		return WorkloadInstanceSubject, nil

	default:
		return "", errors.New("unrecognized reconciler name")
	}

}
