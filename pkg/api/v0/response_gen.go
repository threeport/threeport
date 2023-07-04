// generated by 'threeport-codegen api-version' - do not edit

package v0

// GetStructByObjectType returns and instance of an object for an object type.
func GetStructByObjectType(objectType ObjectType) Object {
	var objStruct Object

	switch objectType {
	case ObjectTypeProfile:
		objStruct = Profile{}
	case ObjectTypeTier:
		objStruct = Tier{}
	case ObjectTypeAwsAccount:
		objStruct = AwsAccount{}
	case ObjectTypeAwsEksClusterDefinition:
		objStruct = AwsEksClusterDefinition{}
	case ObjectTypeAwsEksClusterInstance:
		objStruct = AwsEksClusterInstance{}
	case ObjectTypeAwsRelationalDatabaseDefinition:
		objStruct = AwsRelationalDatabaseDefinition{}
	case ObjectTypeAwsRelationalDatabaseInstance:
		objStruct = AwsRelationalDatabaseInstance{}
	case ObjectTypeClusterDefinition:
		objStruct = ClusterDefinition{}
	case ObjectTypeClusterInstance:
		objStruct = ClusterInstance{}
	case ObjectTypeDomainNameDefinition:
		objStruct = DomainNameDefinition{}
	case ObjectTypeDomainNameInstance:
		objStruct = DomainNameInstance{}
	case ObjectTypeForwardProxyDefinition:
		objStruct = ForwardProxyDefinition{}
	case ObjectTypeForwardProxyInstance:
		objStruct = ForwardProxyInstance{}
	case ObjectTypeGatewayDefinition:
		objStruct = GatewayDefinition{}
	case ObjectTypeGatewayInstance:
		objStruct = GatewayInstance{}
	case ObjectTypeLogBackend:
		objStruct = LogBackend{}
	case ObjectTypeLogStorageDefinition:
		objStruct = LogStorageDefinition{}
	case ObjectTypeLogStorageInstance:
		objStruct = LogStorageInstance{}
	case ObjectTypeWorkloadDefinition:
		objStruct = WorkloadDefinition{}
	case ObjectTypeWorkloadResourceDefinition:
		objStruct = WorkloadResourceDefinition{}
	case ObjectTypeWorkloadInstance:
		objStruct = WorkloadInstance{}
	case ObjectTypeWorkloadResourceInstance:
		objStruct = WorkloadResourceInstance{}
	case ObjectTypeWorkloadEvent:
		objStruct = WorkloadEvent{}

	}

	return objStruct
}

// GetObjectTypeByPath returns the object type based on an API path.
func GetObjectTypeByPath(path string) ObjectType {
	switch path {
	case PathProfiles:
		return ObjectTypeProfile
	case PathTiers:
		return ObjectTypeTier
	case PathAwsAccounts:
		return ObjectTypeAwsAccount
	case PathAwsEksClusterDefinitions:
		return ObjectTypeAwsEksClusterDefinition
	case PathAwsEksClusterInstances:
		return ObjectTypeAwsEksClusterInstance
	case PathAwsRelationalDatabaseDefinitions:
		return ObjectTypeAwsRelationalDatabaseDefinition
	case PathAwsRelationalDatabaseInstances:
		return ObjectTypeAwsRelationalDatabaseInstance
	case PathClusterDefinitions:
		return ObjectTypeClusterDefinition
	case PathClusterInstances:
		return ObjectTypeClusterInstance
	case PathDomainNameDefinitions:
		return ObjectTypeDomainNameDefinition
	case PathDomainNameInstances:
		return ObjectTypeDomainNameInstance
	case PathForwardProxyDefinitions:
		return ObjectTypeForwardProxyDefinition
	case PathForwardProxyInstances:
		return ObjectTypeForwardProxyInstance
	case PathGatewayDefinitions:
		return ObjectTypeGatewayDefinition
	case PathGatewayInstances:
		return ObjectTypeGatewayInstance
	case PathLogBackends:
		return ObjectTypeLogBackend
	case PathLogStorageDefinitions:
		return ObjectTypeLogStorageDefinition
	case PathLogStorageInstances:
		return ObjectTypeLogStorageInstance
	case PathWorkloadDefinitions:
		return ObjectTypeWorkloadDefinition
	case PathWorkloadResourceDefinitions:
		return ObjectTypeWorkloadResourceDefinition
	case PathWorkloadInstances:
		return ObjectTypeWorkloadInstance
	case PathWorkloadResourceInstances:
		return ObjectTypeWorkloadResourceInstance
	case PathWorkloadEvents:
		return ObjectTypeWorkloadEvent

	}

	return ObjectTypeUnknown
}

// GetObjectType returns the object type for an instance of an object.
func GetObjectType(v interface{}) ObjectType {
	switch v.(type) {
	case Profile, *Profile, []Profile:
		return ObjectTypeProfile
	case Tier, *Tier, []Tier:
		return ObjectTypeTier
	case AwsAccount, *AwsAccount, []AwsAccount:
		return ObjectTypeAwsAccount
	case AwsEksClusterDefinition, *AwsEksClusterDefinition, []AwsEksClusterDefinition:
		return ObjectTypeAwsEksClusterDefinition
	case AwsEksClusterInstance, *AwsEksClusterInstance, []AwsEksClusterInstance:
		return ObjectTypeAwsEksClusterInstance
	case AwsRelationalDatabaseDefinition, *AwsRelationalDatabaseDefinition, []AwsRelationalDatabaseDefinition:
		return ObjectTypeAwsRelationalDatabaseDefinition
	case AwsRelationalDatabaseInstance, *AwsRelationalDatabaseInstance, []AwsRelationalDatabaseInstance:
		return ObjectTypeAwsRelationalDatabaseInstance
	case ClusterDefinition, *ClusterDefinition, []ClusterDefinition:
		return ObjectTypeClusterDefinition
	case ClusterInstance, *ClusterInstance, []ClusterInstance:
		return ObjectTypeClusterInstance
	case DomainNameDefinition, *DomainNameDefinition, []DomainNameDefinition:
		return ObjectTypeDomainNameDefinition
	case DomainNameInstance, *DomainNameInstance, []DomainNameInstance:
		return ObjectTypeDomainNameInstance
	case ForwardProxyDefinition, *ForwardProxyDefinition, []ForwardProxyDefinition:
		return ObjectTypeForwardProxyDefinition
	case ForwardProxyInstance, *ForwardProxyInstance, []ForwardProxyInstance:
		return ObjectTypeForwardProxyInstance
	case GatewayDefinition, *GatewayDefinition, []GatewayDefinition:
		return ObjectTypeGatewayDefinition
	case GatewayInstance, *GatewayInstance, []GatewayInstance:
		return ObjectTypeGatewayInstance
	case LogBackend, *LogBackend, []LogBackend:
		return ObjectTypeLogBackend
	case LogStorageDefinition, *LogStorageDefinition, []LogStorageDefinition:
		return ObjectTypeLogStorageDefinition
	case LogStorageInstance, *LogStorageInstance, []LogStorageInstance:
		return ObjectTypeLogStorageInstance
	case WorkloadDefinition, *WorkloadDefinition, []WorkloadDefinition:
		return ObjectTypeWorkloadDefinition
	case WorkloadResourceDefinition, *WorkloadResourceDefinition, []WorkloadResourceDefinition:
		return ObjectTypeWorkloadResourceDefinition
	case WorkloadInstance, *WorkloadInstance, []WorkloadInstance:
		return ObjectTypeWorkloadInstance
	case WorkloadResourceInstance, *WorkloadResourceInstance, []WorkloadResourceInstance:
		return ObjectTypeWorkloadResourceInstance
	case WorkloadEvent, *WorkloadEvent, []WorkloadEvent:
		return ObjectTypeWorkloadEvent

	}

	return ObjectTypeUnknown
}
