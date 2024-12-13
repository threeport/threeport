// generated by 'threeport-sdk gen' - do not edit

package v0

// Tabler allows custom table names for objects in the Threeport database.
type Tabler interface {
	TableName() string
}

// TableName sets the name of the table for the AttachedObjectReference objects in the database.
func (AttachedObjectReference) TableName() string {
	return "v0_attached_object_references"
}

// TableName sets the name of the table for the AwsAccount objects in the database.
func (AwsAccount) TableName() string {
	return "v0_aws_accounts"
}

// TableName sets the name of the table for the AwsEksKubernetesRuntimeDefinition objects in the database.
func (AwsEksKubernetesRuntimeDefinition) TableName() string {
	return "v0_aws_eks_kubernetes_runtime_definitions"
}

// TableName sets the name of the table for the AwsEksKubernetesRuntimeInstance objects in the database.
func (AwsEksKubernetesRuntimeInstance) TableName() string {
	return "v0_aws_eks_kubernetes_runtime_instances"
}

// TableName sets the name of the table for the AwsObjectStorageBucketDefinition objects in the database.
func (AwsObjectStorageBucketDefinition) TableName() string {
	return "v0_aws_object_storage_bucket_definitions"
}

// TableName sets the name of the table for the AwsObjectStorageBucketInstance objects in the database.
func (AwsObjectStorageBucketInstance) TableName() string {
	return "v0_aws_object_storage_bucket_instances"
}

// TableName sets the name of the table for the AwsRelationalDatabaseDefinition objects in the database.
func (AwsRelationalDatabaseDefinition) TableName() string {
	return "v0_aws_relational_database_definitions"
}

// TableName sets the name of the table for the AwsRelationalDatabaseInstance objects in the database.
func (AwsRelationalDatabaseInstance) TableName() string {
	return "v0_aws_relational_database_instances"
}

// TableName sets the name of the table for the ControlPlaneComponent objects in the database.
func (ControlPlaneComponent) TableName() string {
	return "v0_control_plane_components"
}

// TableName sets the name of the table for the KubernetesRuntimeDefinition objects in the database.
func (KubernetesRuntimeDefinition) TableName() string {
	return "v0_kubernetes_runtime_definitions"
}

// TableName sets the name of the table for the KubernetesRuntimeInstance objects in the database.
func (KubernetesRuntimeInstance) TableName() string {
	return "v0_kubernetes_runtime_instances"
}

// TableName sets the name of the table for the Definition objects in the database.
func (Definition) TableName() string {
	return "v0_definitions"
}

// TableName sets the name of the table for the DomainNameDefinition objects in the database.
func (DomainNameDefinition) TableName() string {
	return "v0_domain_name_definitions"
}

// TableName sets the name of the table for the DomainNameInstance objects in the database.
func (DomainNameInstance) TableName() string {
	return "v0_domain_name_instances"
}

// TableName sets the name of the table for the Event objects in the database.
func (Event) TableName() string {
	return "v0_events"
}

// TableName sets the name of the table for the ExtensionApi objects in the database.
func (ExtensionApi) TableName() string {
	return "v0_extension_apis"
}

// TableName sets the name of the table for the ExtensionApiRoute objects in the database.
func (ExtensionApiRoute) TableName() string {
	return "v0_extension_api_routes"
}

// TableName sets the name of the table for the GatewayDefinition objects in the database.
func (GatewayDefinition) TableName() string {
	return "v0_gateway_definitions"
}

// TableName sets the name of the table for the GatewayHttpPort objects in the database.
func (GatewayHttpPort) TableName() string {
	return "v0_gateway_http_ports"
}

// TableName sets the name of the table for the GatewayInstance objects in the database.
func (GatewayInstance) TableName() string {
	return "v0_gateway_instances"
}

// TableName sets the name of the table for the GatewayTcpPort objects in the database.
func (GatewayTcpPort) TableName() string {
	return "v0_gateway_tcp_ports"
}

// TableName sets the name of the table for the HelmWorkloadDefinition objects in the database.
func (HelmWorkloadDefinition) TableName() string {
	return "v0_helm_workload_definitions"
}

// TableName sets the name of the table for the HelmWorkloadInstance objects in the database.
func (HelmWorkloadInstance) TableName() string {
	return "v0_helm_workload_instances"
}

// TableName sets the name of the table for the Instance objects in the database.
func (Instance) TableName() string {
	return "v0_instances"
}

// TableName sets the name of the table for the ControlPlaneDefinition objects in the database.
func (ControlPlaneDefinition) TableName() string {
	return "v0_control_plane_definitions"
}

// TableName sets the name of the table for the ControlPlaneInstance objects in the database.
func (ControlPlaneInstance) TableName() string {
	return "v0_control_plane_instances"
}

// TableName sets the name of the table for the LogBackend objects in the database.
func (LogBackend) TableName() string {
	return "v0_log_backends"
}

// TableName sets the name of the table for the LogStorageDefinition objects in the database.
func (LogStorageDefinition) TableName() string {
	return "v0_log_storage_definitions"
}

// TableName sets the name of the table for the LogStorageInstance objects in the database.
func (LogStorageInstance) TableName() string {
	return "v0_log_storage_instances"
}

// TableName sets the name of the table for the LoggingDefinition objects in the database.
func (LoggingDefinition) TableName() string {
	return "v0_logging_definitions"
}

// TableName sets the name of the table for the LoggingInstance objects in the database.
func (LoggingInstance) TableName() string {
	return "v0_logging_instances"
}

// TableName sets the name of the table for the MetricsDefinition objects in the database.
func (MetricsDefinition) TableName() string {
	return "v0_metrics_definitions"
}

// TableName sets the name of the table for the MetricsInstance objects in the database.
func (MetricsInstance) TableName() string {
	return "v0_metrics_instances"
}

// TableName sets the name of the table for the ObservabilityDashboardDefinition objects in the database.
func (ObservabilityDashboardDefinition) TableName() string {
	return "v0_observability_dashboard_definitions"
}

// TableName sets the name of the table for the ObservabilityDashboardInstance objects in the database.
func (ObservabilityDashboardInstance) TableName() string {
	return "v0_observability_dashboard_instances"
}

// TableName sets the name of the table for the ObservabilityStackDefinition objects in the database.
func (ObservabilityStackDefinition) TableName() string {
	return "v0_observability_stack_definitions"
}

// TableName sets the name of the table for the ObservabilityStackInstance objects in the database.
func (ObservabilityStackInstance) TableName() string {
	return "v0_observability_stack_instances"
}

// TableName sets the name of the table for the Profile objects in the database.
func (Profile) TableName() string {
	return "v0_profiles"
}

// TableName sets the name of the table for the SecretDefinition objects in the database.
func (SecretDefinition) TableName() string {
	return "v0_secret_definitions"
}

// TableName sets the name of the table for the SecretInstance objects in the database.
func (SecretInstance) TableName() string {
	return "v0_secret_instances"
}

// TableName sets the name of the table for the TerraformDefinition objects in the database.
func (TerraformDefinition) TableName() string {
	return "v0_terraform_definitions"
}

// TableName sets the name of the table for the TerraformInstance objects in the database.
func (TerraformInstance) TableName() string {
	return "v0_terraform_instances"
}

// TableName sets the name of the table for the Tier objects in the database.
func (Tier) TableName() string {
	return "v0_tiers"
}

// TableName sets the name of the table for the WorkloadDefinition objects in the database.
func (WorkloadDefinition) TableName() string {
	return "v0_workload_definitions"
}

// TableName sets the name of the table for the WorkloadEvent objects in the database.
func (WorkloadEvent) TableName() string {
	return "v0_workload_events"
}

// TableName sets the name of the table for the WorkloadInstance objects in the database.
func (WorkloadInstance) TableName() string {
	return "v0_workload_instances"
}

// TableName sets the name of the table for the WorkloadResourceDefinition objects in the database.
func (WorkloadResourceDefinition) TableName() string {
	return "v0_workload_resource_definitions"
}

// TableName sets the name of the table for the WorkloadResourceInstance objects in the database.
func (WorkloadResourceInstance) TableName() string {
	return "v0_workload_resource_instances"
}
