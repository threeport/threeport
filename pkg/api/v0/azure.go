// originally generated by 'threeport-sdk create' for API object
// scaffolding but will not be re-generated - intended for modification

package v0

// AzureAccount is a user account with the Azure service provider.
type AzureAccount struct {
	Common `mapstructure:",squash" swaggerignore:"true"`

	// The unique name of an AKS account.
	Name *string `json:"Name,omitempty" query:"name" gorm:"not null" validate:"required"`

	// The credentials for the Azure account.
	Credentials *string `json:"Credentials,omitempty" query:"credentials" gorm:"not null" validate:"required" encrypt:"true"`

	// If true, this is the Azure Account used if none specified in a definition.
	DefaultAccount *bool `json:"DefaultAccount,omitempty" query:"defaultaccount" gorm:"default:false" validate:"optional"`

	// The region to use for Azure managed services if not specified.
	DefaultRegion *string `json:"DefaultRegion,omitempty" query:"defaultregion" gorm:"not null" validate:"required"`

	// The cluster instances deployed in this Azure account.
	AzureAksKubernetesRuntimeDefinition []*AzureAksKubernetesRuntimeDefinition `json:"AzureAksKubernetesRuntimeDefinition,omitempty" validate:"optional,association"`
}

// AzureAksKubernetesRuntimeDefinition provides the configuration for AKS cluster instances.
type AzureAksKubernetesRuntimeDefinition struct {
	Common     `mapstructure:",squash" swaggerignore:"true"`
	Definition `mapstructure:",squash"`

	// The Azure account in which the EKS cluster is provisioned.
	AzureAccountID *uint `json:"AzureAccountID,omitempty" query:"azureaccountid" gorm:"not null" validate:"required"`

	// TODO: add fields for region limitations
	// RegionsAllowed
	// RegionsForbidden

	// The kubernetes runtime definition for an AKS cluster in Azure.
	KubernetesRuntimeDefinitionID *uint `json:"KubernetesRuntimeDefinitionID,omitempty" query:"kubernetesruntimedefinitionid" gorm:"not null" validate:"required"`

	// The associated aks k8s runtime instances
	AzureAksKubernetesRuntimeInstances []*AzureAksKubernetesRuntimeInstance `json:"AzureAksKubernetesRuntimeInstances,omitempty" validate:"optional,association"`
}

// AzureAksKubernetesRuntimeInstance is a deployed instance of an AKS cluster.
type AzureAksKubernetesRuntimeInstance struct {
	Common         `mapstructure:",squash" swaggerignore:"true"`
	Reconciliation `mapstructure:",squash"`
	Instance       `mapstructure:",squash"`

	// The runtime definition associated with this instance
	AzureAksKubernetesRuntimeDefinitionID *uint `gorm:"not null" json:"AzureAksKubernetesRuntimeDefinitionID,omitempty" query:"azureakskubernetesruntimedefinitionid" validate:"required"`

	// The Azure Region in which the cluster is provisioned.
	Region *string `json:"Region,omitempty" query:"region" validate:"optional"`

	// The kubernetes runtime instance associated with the AWS EKS cluster.
	KubernetesRuntimeInstanceID *uint `json:"KubernetesRuntimeInstanceID,omitempty" query:"kubernetesruntimeinstanceid" gorm:"not null" validate:"required"`
}

// AzureRelationalDatabaseDefinition is the configuration for an Relational database instance
// provided by Azure that is used by a workload.
type AzureRelationalDatabaseDefinition struct {
	Common     `mapstructure:",squash" swaggerignore:"true"`
	Definition `mapstructure:",squash"`

	// The Azure account in which the database is provisioned.
	AzureAccountID *uint `json:"AzureAccountID,omitempty" query:"azureaccountid" gorm:"not null" validate:"required"`

	// The database engine for the instance.  One of:
	// * mysql
	// * postgres
	// * mariadb
	Engine *string `json:"Engine,omitempty" query:"engine" gorm:"not null" validate:"required"`

	// The version of the database engine for the instance.
	EngineVersion *string `json:"EngineVersion,omitempty" query:"engineversion" gorm:"not null" validate:"required"`

	// The name of the database that will be used by the client workload.
	DatabaseName *string `json:"DatabaseName,omitempty" query:"databasename" gorm:"not null" validate:"required"`

	// The port to use to connect to the database.
	DatabasePort *int `json:"DatabasePort,omitempty" query:"databaseport" gorm:"not null" validate:"required"`

	// The number of days to retain database backups for.
	BackupDays *int `json:"BackupDays,omitempty" query:"BackupDays" gorm:"default: 0" validate:"optional"`

	// The amount of compute capacity to use for the database virtual machine.
	MachineSize *string `json:"MachineSize,omitempty" query:"machinesize" gorm:"not null" validate:"required"`

	// The amount of storage in Gb to allocate for the database.
	StorageGb *int `json:"StorageGb,omitempty" query:"storagegb" gorm:"not null" validate:"required"`

	// The name of the Kubernetes secret that will be attached to the
	// running workload from which database connection configuration will be
	// supplied.  This secret name must be referred to in the Kubernetes
	// manifest, .e.g Deployment, for the workload.
	WorkloadSecretName *string `json:"WorkloadSecretName,omitempty" query:"WorkloadSecretName" gorm:"not null" validate:"required"`

	// The associated relational database instances that are derived from this definition.
	AzureRelationalDatabaseInstances []*AzureRelationalDatabaseInstance `json:"AwsRelationalDatabaseInstances,omitempty" validate:"optional,association"`
}

// AzureRelationalDatabaseInstance is a deployed instance of an Relational Database in Azure.
type AzureRelationalDatabaseInstance struct {
	Common         `swaggerignore:"true" mapstructure:",squash"`
	Instance       `mapstructure:",squash"`
	Reconciliation `mapstructure:",squash"`

	// The definition that configures this instance.
	AzureRelationalDatabaseDefinitionID *uint `json:"AzureRelationalDatabaseDefinitionID,omitempty" query:"azurerelationaldatabasedefinitionid" gorm:"not null" validate:"required"`

	// The ID of the workload instance that the database instance serves.
	WorkloadInstanceID *uint `json:"WorkloadInstanceID,omitempty" query:"workloadinstanceid" gorm:"not null" validate:"required"`
}
