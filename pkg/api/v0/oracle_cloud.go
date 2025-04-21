package v0

// OracleCloudAccount represents an Oracle Cloud account configuration.
type OracleCloudAccount struct {
	Common
	Name           *string `json:"name,omitempty"`
	TenancyID      *string `json:"tenancyId,omitempty"`
	CompartmentID  *string `json:"compartmentId,omitempty"`
	DefaultRegion  *string `json:"defaultRegion,omitempty"`
	DefaultAccount *bool   `json:"defaultAccount,omitempty"`
}

// OracleCloudOKEKubernetesRuntimeDefinition represents the definition for an Oracle Cloud OKE
// Kubernetes runtime.
type OracleCloudOKEKubernetesRuntimeDefinition struct {
	Definition
	OracleCloudAccountID          *uint   `json:"oracleCloudAccountId,omitempty"`
	AvailabilityDomainCount       *int32  `json:"availabilityDomainCount,omitempty"`
	WorkerNodeShape               *string `json:"workerNodeShape,omitempty"`
	WorkerNodeInitialCount        *int32  `json:"workerNodeInitialCount,omitempty"`
	WorkerNodeMinCount            *int32  `json:"workerNodeMinCount,omitempty"`
	WorkerNodeMaxCount            *int32  `json:"workerNodeMaxCount,omitempty"`
	KubernetesRuntimeDefinitionID *uint   `json:"kubernetesRuntimeDefinitionId,omitempty"`
}

// OracleCloudOKEKubernetesRuntimeInstance represents an instance of an Oracle Cloud OKE
// Kubernetes runtime.
type OracleCloudOKEKubernetesRuntimeInstance struct {
	Instance
	Region                                      *string `json:"region,omitempty"`
	KubernetesRuntimeInstanceID                 *uint   `json:"kubernetesRuntimeInstanceId,omitempty"`
	OracleCloudOKEKubernetesRuntimeDefinitionID *uint   `json:"oracleCloudOkeKubernetesRuntimeDefinitionId,omitempty"`
}
