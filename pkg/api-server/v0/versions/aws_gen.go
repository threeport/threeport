// generated by 'threeport-sdk gen' - do not edit

package versions

import (
	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	apiserver_v0 "github.com/threeport/threeport/pkg/api-server/v0"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	"reflect"
)

// AddAwsAccountVersions adds field validation info and adds it
// to the REST API versions.
func AddAwsAccountVersions() {
	apiserver_v0.AwsAccountTaggedFields[apiserver_lib.TagNameValidate] = &apiserver_lib.FieldsByTag{
		Optional:             []string{},
		OptionalAssociations: []string{},
		Required:             []string{},
		TagName:              apiserver_lib.TagNameValidate,
	}

	// parse struct and populate the FieldsByTag object
	apiserver_lib.ParseStruct(
		apiserver_lib.TagNameValidate,
		reflect.ValueOf(new(api_v0.AwsAccount)),
		"",
		apiserver_lib.Translate,
		apiserver_v0.AwsAccountTaggedFields,
	)

	// create a version object which contains the object name and versions
	versionObj := apiserver_lib.VersionObject{
		Object:  string(api_v0.ObjectTypeAwsAccount),
		Version: "v0",
	}

	// add the object tagged fields to the global tagged fields map
	apiserver_lib.ObjectTaggedFields[versionObj] = apiserver_v0.AwsAccountTaggedFields[apiserver_lib.TagNameValidate]

	// add the object tagged fields to the rest API version
	apiserver_lib.AddObjectVersion(versionObj)
}

// AddAwsEksKubernetesRuntimeDefinitionVersions adds field validation info and adds it
// to the REST API versions.
func AddAwsEksKubernetesRuntimeDefinitionVersions() {
	apiserver_v0.AwsEksKubernetesRuntimeDefinitionTaggedFields[apiserver_lib.TagNameValidate] = &apiserver_lib.FieldsByTag{
		Optional:             []string{},
		OptionalAssociations: []string{},
		Required:             []string{},
		TagName:              apiserver_lib.TagNameValidate,
	}

	// parse struct and populate the FieldsByTag object
	apiserver_lib.ParseStruct(
		apiserver_lib.TagNameValidate,
		reflect.ValueOf(new(api_v0.AwsEksKubernetesRuntimeDefinition)),
		"",
		apiserver_lib.Translate,
		apiserver_v0.AwsEksKubernetesRuntimeDefinitionTaggedFields,
	)

	// create a version object which contains the object name and versions
	versionObj := apiserver_lib.VersionObject{
		Object:  string(api_v0.ObjectTypeAwsEksKubernetesRuntimeDefinition),
		Version: "v0",
	}

	// add the object tagged fields to the global tagged fields map
	apiserver_lib.ObjectTaggedFields[versionObj] = apiserver_v0.AwsEksKubernetesRuntimeDefinitionTaggedFields[apiserver_lib.TagNameValidate]

	// add the object tagged fields to the rest API version
	apiserver_lib.AddObjectVersion(versionObj)
}

// AddAwsEksKubernetesRuntimeInstanceVersions adds field validation info and adds it
// to the REST API versions.
func AddAwsEksKubernetesRuntimeInstanceVersions() {
	apiserver_v0.AwsEksKubernetesRuntimeInstanceTaggedFields[apiserver_lib.TagNameValidate] = &apiserver_lib.FieldsByTag{
		Optional:             []string{},
		OptionalAssociations: []string{},
		Required:             []string{},
		TagName:              apiserver_lib.TagNameValidate,
	}

	// parse struct and populate the FieldsByTag object
	apiserver_lib.ParseStruct(
		apiserver_lib.TagNameValidate,
		reflect.ValueOf(new(api_v0.AwsEksKubernetesRuntimeInstance)),
		"",
		apiserver_lib.Translate,
		apiserver_v0.AwsEksKubernetesRuntimeInstanceTaggedFields,
	)

	// create a version object which contains the object name and versions
	versionObj := apiserver_lib.VersionObject{
		Object:  string(api_v0.ObjectTypeAwsEksKubernetesRuntimeInstance),
		Version: "v0",
	}

	// add the object tagged fields to the global tagged fields map
	apiserver_lib.ObjectTaggedFields[versionObj] = apiserver_v0.AwsEksKubernetesRuntimeInstanceTaggedFields[apiserver_lib.TagNameValidate]

	// add the object tagged fields to the rest API version
	apiserver_lib.AddObjectVersion(versionObj)
}

// AddAwsObjectStorageBucketDefinitionVersions adds field validation info and adds it
// to the REST API versions.
func AddAwsObjectStorageBucketDefinitionVersions() {
	apiserver_v0.AwsObjectStorageBucketDefinitionTaggedFields[apiserver_lib.TagNameValidate] = &apiserver_lib.FieldsByTag{
		Optional:             []string{},
		OptionalAssociations: []string{},
		Required:             []string{},
		TagName:              apiserver_lib.TagNameValidate,
	}

	// parse struct and populate the FieldsByTag object
	apiserver_lib.ParseStruct(
		apiserver_lib.TagNameValidate,
		reflect.ValueOf(new(api_v0.AwsObjectStorageBucketDefinition)),
		"",
		apiserver_lib.Translate,
		apiserver_v0.AwsObjectStorageBucketDefinitionTaggedFields,
	)

	// create a version object which contains the object name and versions
	versionObj := apiserver_lib.VersionObject{
		Object:  string(api_v0.ObjectTypeAwsObjectStorageBucketDefinition),
		Version: "v0",
	}

	// add the object tagged fields to the global tagged fields map
	apiserver_lib.ObjectTaggedFields[versionObj] = apiserver_v0.AwsObjectStorageBucketDefinitionTaggedFields[apiserver_lib.TagNameValidate]

	// add the object tagged fields to the rest API version
	apiserver_lib.AddObjectVersion(versionObj)
}

// AddAwsObjectStorageBucketInstanceVersions adds field validation info and adds it
// to the REST API versions.
func AddAwsObjectStorageBucketInstanceVersions() {
	apiserver_v0.AwsObjectStorageBucketInstanceTaggedFields[apiserver_lib.TagNameValidate] = &apiserver_lib.FieldsByTag{
		Optional:             []string{},
		OptionalAssociations: []string{},
		Required:             []string{},
		TagName:              apiserver_lib.TagNameValidate,
	}

	// parse struct and populate the FieldsByTag object
	apiserver_lib.ParseStruct(
		apiserver_lib.TagNameValidate,
		reflect.ValueOf(new(api_v0.AwsObjectStorageBucketInstance)),
		"",
		apiserver_lib.Translate,
		apiserver_v0.AwsObjectStorageBucketInstanceTaggedFields,
	)

	// create a version object which contains the object name and versions
	versionObj := apiserver_lib.VersionObject{
		Object:  string(api_v0.ObjectTypeAwsObjectStorageBucketInstance),
		Version: "v0",
	}

	// add the object tagged fields to the global tagged fields map
	apiserver_lib.ObjectTaggedFields[versionObj] = apiserver_v0.AwsObjectStorageBucketInstanceTaggedFields[apiserver_lib.TagNameValidate]

	// add the object tagged fields to the rest API version
	apiserver_lib.AddObjectVersion(versionObj)
}

// AddAwsRelationalDatabaseDefinitionVersions adds field validation info and adds it
// to the REST API versions.
func AddAwsRelationalDatabaseDefinitionVersions() {
	apiserver_v0.AwsRelationalDatabaseDefinitionTaggedFields[apiserver_lib.TagNameValidate] = &apiserver_lib.FieldsByTag{
		Optional:             []string{},
		OptionalAssociations: []string{},
		Required:             []string{},
		TagName:              apiserver_lib.TagNameValidate,
	}

	// parse struct and populate the FieldsByTag object
	apiserver_lib.ParseStruct(
		apiserver_lib.TagNameValidate,
		reflect.ValueOf(new(api_v0.AwsRelationalDatabaseDefinition)),
		"",
		apiserver_lib.Translate,
		apiserver_v0.AwsRelationalDatabaseDefinitionTaggedFields,
	)

	// create a version object which contains the object name and versions
	versionObj := apiserver_lib.VersionObject{
		Object:  string(api_v0.ObjectTypeAwsRelationalDatabaseDefinition),
		Version: "v0",
	}

	// add the object tagged fields to the global tagged fields map
	apiserver_lib.ObjectTaggedFields[versionObj] = apiserver_v0.AwsRelationalDatabaseDefinitionTaggedFields[apiserver_lib.TagNameValidate]

	// add the object tagged fields to the rest API version
	apiserver_lib.AddObjectVersion(versionObj)
}

// AddAwsRelationalDatabaseInstanceVersions adds field validation info and adds it
// to the REST API versions.
func AddAwsRelationalDatabaseInstanceVersions() {
	apiserver_v0.AwsRelationalDatabaseInstanceTaggedFields[apiserver_lib.TagNameValidate] = &apiserver_lib.FieldsByTag{
		Optional:             []string{},
		OptionalAssociations: []string{},
		Required:             []string{},
		TagName:              apiserver_lib.TagNameValidate,
	}

	// parse struct and populate the FieldsByTag object
	apiserver_lib.ParseStruct(
		apiserver_lib.TagNameValidate,
		reflect.ValueOf(new(api_v0.AwsRelationalDatabaseInstance)),
		"",
		apiserver_lib.Translate,
		apiserver_v0.AwsRelationalDatabaseInstanceTaggedFields,
	)

	// create a version object which contains the object name and versions
	versionObj := apiserver_lib.VersionObject{
		Object:  string(api_v0.ObjectTypeAwsRelationalDatabaseInstance),
		Version: "v0",
	}

	// add the object tagged fields to the global tagged fields map
	apiserver_lib.ObjectTaggedFields[versionObj] = apiserver_v0.AwsRelationalDatabaseInstanceTaggedFields[apiserver_lib.TagNameValidate]

	// add the object tagged fields to the rest API version
	apiserver_lib.AddObjectVersion(versionObj)
}
