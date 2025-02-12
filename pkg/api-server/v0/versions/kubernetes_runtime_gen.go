// generated by 'threeport-sdk gen' - do not edit

package versions

import (
	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	apiserver_v0 "github.com/threeport/threeport/pkg/api-server/v0"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	"reflect"
)

// AddKubernetesRuntimeDefinitionVersions adds field validation info and adds it
// to the REST API versions.
func AddKubernetesRuntimeDefinitionVersions() {
	apiserver_v0.KubernetesRuntimeDefinitionTaggedFields[apiserver_lib.TagNameValidate] = &apiserver_lib.FieldsByTag{
		Optional:             []string{},
		OptionalAssociations: []string{},
		Required:             []string{},
		TagName:              apiserver_lib.TagNameValidate,
	}

	// parse struct and populate the FieldsByTag object
	apiserver_lib.ParseStruct(
		apiserver_lib.TagNameValidate,
		reflect.ValueOf(new(api_v0.KubernetesRuntimeDefinition)),
		"",
		apiserver_lib.Translate,
		apiserver_v0.KubernetesRuntimeDefinitionTaggedFields,
	)

	// create a version object which contains the object name and versions
	versionObj := apiserver_lib.VersionObject{
		Object:  string(api_v0.ObjectTypeKubernetesRuntimeDefinition),
		Version: "v0",
	}

	// add the object tagged fields to the global tagged fields map
	apiserver_lib.ObjectTaggedFields[versionObj] = apiserver_v0.KubernetesRuntimeDefinitionTaggedFields[apiserver_lib.TagNameValidate]

	// add the object tagged fields to the rest API version
	apiserver_lib.AddObjectVersion(versionObj)
}

// AddKubernetesRuntimeInstanceVersions adds field validation info and adds it
// to the REST API versions.
func AddKubernetesRuntimeInstanceVersions() {
	apiserver_v0.KubernetesRuntimeInstanceTaggedFields[apiserver_lib.TagNameValidate] = &apiserver_lib.FieldsByTag{
		Optional:             []string{},
		OptionalAssociations: []string{},
		Required:             []string{},
		TagName:              apiserver_lib.TagNameValidate,
	}

	// parse struct and populate the FieldsByTag object
	apiserver_lib.ParseStruct(
		apiserver_lib.TagNameValidate,
		reflect.ValueOf(new(api_v0.KubernetesRuntimeInstance)),
		"",
		apiserver_lib.Translate,
		apiserver_v0.KubernetesRuntimeInstanceTaggedFields,
	)

	// create a version object which contains the object name and versions
	versionObj := apiserver_lib.VersionObject{
		Object:  string(api_v0.ObjectTypeKubernetesRuntimeInstance),
		Version: "v0",
	}

	// add the object tagged fields to the global tagged fields map
	apiserver_lib.ObjectTaggedFields[versionObj] = apiserver_v0.KubernetesRuntimeInstanceTaggedFields[apiserver_lib.TagNameValidate]

	// add the object tagged fields to the rest API version
	apiserver_lib.AddObjectVersion(versionObj)
}
