// generated by 'threeport-codegen api-model' - do not edit

package versions

import (
	api "github.com/threeport/threeport/pkg/api"
	iapi "github.com/threeport/threeport/pkg/api-server/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	"reflect"
)

// AddAwsAccountVersions adds field validation info and adds it
// to the REST API versions.
func AddAwsAccountVersions() {
	iapi.AwsAccountTaggedFields[iapi.TagNameValidate] = &iapi.FieldsByTag{
		Optional:             []string{},
		OptionalAssociations: []string{},
		Required:             []string{},
		TagName:              iapi.TagNameValidate,
	}

	// parse struct and populate the FieldsByTag object
	iapi.ParseStruct(
		iapi.TagNameValidate,
		reflect.ValueOf(new(v0.AwsAccount)),
		"",
		iapi.Translate,
		iapi.AwsAccountTaggedFields,
	)

	// create a version object which contains the object name and versions
	versionObj := iapi.VersionObject{
		Object:  string(v0.ObjectTypeAwsAccount),
		Version: iapi.V0,
	}

	// add the object tagged fields to the global tagged fields map
	iapi.ObjectTaggedFields[versionObj] = iapi.AwsAccountTaggedFields[iapi.TagNameValidate]

	// add the object tagged fields to the rest API version
	api.AddRestApiVersion(versionObj)
}

// AddAwsEksKubernetesRuntimeDefinitionVersions adds field validation info and adds it
// to the REST API versions.
func AddAwsEksKubernetesRuntimeDefinitionVersions() {
	iapi.AwsEksKubernetesRuntimeDefinitionTaggedFields[iapi.TagNameValidate] = &iapi.FieldsByTag{
		Optional:             []string{},
		OptionalAssociations: []string{},
		Required:             []string{},
		TagName:              iapi.TagNameValidate,
	}

	// parse struct and populate the FieldsByTag object
	iapi.ParseStruct(
		iapi.TagNameValidate,
		reflect.ValueOf(new(v0.AwsEksKubernetesRuntimeDefinition)),
		"",
		iapi.Translate,
		iapi.AwsEksKubernetesRuntimeDefinitionTaggedFields,
	)

	// create a version object which contains the object name and versions
	versionObj := iapi.VersionObject{
		Object:  string(v0.ObjectTypeAwsEksKubernetesRuntimeDefinition),
		Version: iapi.V0,
	}

	// add the object tagged fields to the global tagged fields map
	iapi.ObjectTaggedFields[versionObj] = iapi.AwsEksKubernetesRuntimeDefinitionTaggedFields[iapi.TagNameValidate]

	// add the object tagged fields to the rest API version
	api.AddRestApiVersion(versionObj)
}

// AddAwsEksKubernetesRuntimeInstanceVersions adds field validation info and adds it
// to the REST API versions.
func AddAwsEksKubernetesRuntimeInstanceVersions() {
	iapi.AwsEksKubernetesRuntimeInstanceTaggedFields[iapi.TagNameValidate] = &iapi.FieldsByTag{
		Optional:             []string{},
		OptionalAssociations: []string{},
		Required:             []string{},
		TagName:              iapi.TagNameValidate,
	}

	// parse struct and populate the FieldsByTag object
	iapi.ParseStruct(
		iapi.TagNameValidate,
		reflect.ValueOf(new(v0.AwsEksKubernetesRuntimeInstance)),
		"",
		iapi.Translate,
		iapi.AwsEksKubernetesRuntimeInstanceTaggedFields,
	)

	// create a version object which contains the object name and versions
	versionObj := iapi.VersionObject{
		Object:  string(v0.ObjectTypeAwsEksKubernetesRuntimeInstance),
		Version: iapi.V0,
	}

	// add the object tagged fields to the global tagged fields map
	iapi.ObjectTaggedFields[versionObj] = iapi.AwsEksKubernetesRuntimeInstanceTaggedFields[iapi.TagNameValidate]

	// add the object tagged fields to the rest API version
	api.AddRestApiVersion(versionObj)
}

// AddAwsRelationalDatabaseDefinitionVersions adds field validation info and adds it
// to the REST API versions.
func AddAwsRelationalDatabaseDefinitionVersions() {
	iapi.AwsRelationalDatabaseDefinitionTaggedFields[iapi.TagNameValidate] = &iapi.FieldsByTag{
		Optional:             []string{},
		OptionalAssociations: []string{},
		Required:             []string{},
		TagName:              iapi.TagNameValidate,
	}

	// parse struct and populate the FieldsByTag object
	iapi.ParseStruct(
		iapi.TagNameValidate,
		reflect.ValueOf(new(v0.AwsRelationalDatabaseDefinition)),
		"",
		iapi.Translate,
		iapi.AwsRelationalDatabaseDefinitionTaggedFields,
	)

	// create a version object which contains the object name and versions
	versionObj := iapi.VersionObject{
		Object:  string(v0.ObjectTypeAwsRelationalDatabaseDefinition),
		Version: iapi.V0,
	}

	// add the object tagged fields to the global tagged fields map
	iapi.ObjectTaggedFields[versionObj] = iapi.AwsRelationalDatabaseDefinitionTaggedFields[iapi.TagNameValidate]

	// add the object tagged fields to the rest API version
	api.AddRestApiVersion(versionObj)
}

// AddAwsRelationalDatabaseInstanceVersions adds field validation info and adds it
// to the REST API versions.
func AddAwsRelationalDatabaseInstanceVersions() {
	iapi.AwsRelationalDatabaseInstanceTaggedFields[iapi.TagNameValidate] = &iapi.FieldsByTag{
		Optional:             []string{},
		OptionalAssociations: []string{},
		Required:             []string{},
		TagName:              iapi.TagNameValidate,
	}

	// parse struct and populate the FieldsByTag object
	iapi.ParseStruct(
		iapi.TagNameValidate,
		reflect.ValueOf(new(v0.AwsRelationalDatabaseInstance)),
		"",
		iapi.Translate,
		iapi.AwsRelationalDatabaseInstanceTaggedFields,
	)

	// create a version object which contains the object name and versions
	versionObj := iapi.VersionObject{
		Object:  string(v0.ObjectTypeAwsRelationalDatabaseInstance),
		Version: iapi.V0,
	}

	// add the object tagged fields to the global tagged fields map
	iapi.ObjectTaggedFields[versionObj] = iapi.AwsRelationalDatabaseInstanceTaggedFields[iapi.TagNameValidate]

	// add the object tagged fields to the rest API version
	api.AddRestApiVersion(versionObj)
}

// AddAwsObjectStorageBucketDefinitionVersions adds field validation info and adds it
// to the REST API versions.
func AddAwsObjectStorageBucketDefinitionVersions() {
	iapi.AwsObjectStorageBucketDefinitionTaggedFields[iapi.TagNameValidate] = &iapi.FieldsByTag{
		Optional:             []string{},
		OptionalAssociations: []string{},
		Required:             []string{},
		TagName:              iapi.TagNameValidate,
	}

	// parse struct and populate the FieldsByTag object
	iapi.ParseStruct(
		iapi.TagNameValidate,
		reflect.ValueOf(new(v0.AwsObjectStorageBucketDefinition)),
		"",
		iapi.Translate,
		iapi.AwsObjectStorageBucketDefinitionTaggedFields,
	)

	// create a version object which contains the object name and versions
	versionObj := iapi.VersionObject{
		Object:  string(v0.ObjectTypeAwsObjectStorageBucketDefinition),
		Version: iapi.V0,
	}

	// add the object tagged fields to the global tagged fields map
	iapi.ObjectTaggedFields[versionObj] = iapi.AwsObjectStorageBucketDefinitionTaggedFields[iapi.TagNameValidate]

	// add the object tagged fields to the rest API version
	api.AddRestApiVersion(versionObj)
}

// AddAwsObjectStorageBucketInstanceVersions adds field validation info and adds it
// to the REST API versions.
func AddAwsObjectStorageBucketInstanceVersions() {
	iapi.AwsObjectStorageBucketInstanceTaggedFields[iapi.TagNameValidate] = &iapi.FieldsByTag{
		Optional:             []string{},
		OptionalAssociations: []string{},
		Required:             []string{},
		TagName:              iapi.TagNameValidate,
	}

	// parse struct and populate the FieldsByTag object
	iapi.ParseStruct(
		iapi.TagNameValidate,
		reflect.ValueOf(new(v0.AwsObjectStorageBucketInstance)),
		"",
		iapi.Translate,
		iapi.AwsObjectStorageBucketInstanceTaggedFields,
	)

	// create a version object which contains the object name and versions
	versionObj := iapi.VersionObject{
		Object:  string(v0.ObjectTypeAwsObjectStorageBucketInstance),
		Version: iapi.V0,
	}

	// add the object tagged fields to the global tagged fields map
	iapi.ObjectTaggedFields[versionObj] = iapi.AwsObjectStorageBucketInstanceTaggedFields[iapi.TagNameValidate]

	// add the object tagged fields to the rest API version
	api.AddRestApiVersion(versionObj)
}
