// +threeport-codegen route-exclude
// +threeport-codegen database-exclude
package v0

type APIObject interface {
	GetID() uint
	NotificationPayload(requeue bool, lastDelay int64) (*[]byte, error)
}

type WorkloadDependency interface {
	APIObject
	GetStatus()
}

func GetAPIObjectByObjectType(objectType ObjectType) APIObject {
	var objStruct APIObject

	switch objectType {
	//case ObjectTypeCompany:
	//	objStruct = &Company{}
	//case ObjectTypeUser:
	//	objStruct = &User{}
	//case ObjectTypeWorkloadCluster:
	//	objStruct = &WorkloadCluster{}
	//case ObjectTypeWorkloadServiceDependency:
	//	objStruct = &WorkloadServiceDependency{}
	case ObjectTypeWorkloadDefinition:
		objStruct = &WorkloadDefinition{}
	case ObjectTypeWorkloadResourceDefinition:
		objStruct = &WorkloadResourceDefinition{}
	case ObjectTypeWorkloadInstance:
		objStruct = &WorkloadInstance{}
	}
	return objStruct
}
