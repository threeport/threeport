// generated by 'threeport-codegen api-model' - do not edit

package versions

import (
	api "github.com/threeport/threeport/pkg/api"
	iapi "github.com/threeport/threeport/pkg/api-server/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	"reflect"
)

// AddMetricsDefinitionVersions adds field validation info and adds it
// to the REST API versions.
func AddMetricsDefinitionVersions() {
	iapi.MetricsDefinitionTaggedFields[iapi.TagNameValidate] = &iapi.FieldsByTag{
		Optional:             []string{},
		OptionalAssociations: []string{},
		Required:             []string{},
		TagName:              iapi.TagNameValidate,
	}

	// parse struct and populate the FieldsByTag object
	iapi.ParseStruct(
		iapi.TagNameValidate,
		reflect.ValueOf(new(v0.MetricsDefinition)),
		"",
		iapi.Translate,
		iapi.MetricsDefinitionTaggedFields,
	)

	// create a version object which contains the object name and versions
	versionObj := iapi.VersionObject{
		Object:  string(v0.ObjectTypeMetricsDefinition),
		Version: iapi.V0,
	}

	// add the object tagged fields to the global tagged fields map
	iapi.ObjectTaggedFields[versionObj] = iapi.MetricsDefinitionTaggedFields[iapi.TagNameValidate]

	// add the object tagged fields to the rest API version
	api.AddRestApiVersion(versionObj)
}

// AddMetricsInstanceVersions adds field validation info and adds it
// to the REST API versions.
func AddMetricsInstanceVersions() {
	iapi.MetricsInstanceTaggedFields[iapi.TagNameValidate] = &iapi.FieldsByTag{
		Optional:             []string{},
		OptionalAssociations: []string{},
		Required:             []string{},
		TagName:              iapi.TagNameValidate,
	}

	// parse struct and populate the FieldsByTag object
	iapi.ParseStruct(
		iapi.TagNameValidate,
		reflect.ValueOf(new(v0.MetricsInstance)),
		"",
		iapi.Translate,
		iapi.MetricsInstanceTaggedFields,
	)

	// create a version object which contains the object name and versions
	versionObj := iapi.VersionObject{
		Object:  string(v0.ObjectTypeMetricsInstance),
		Version: iapi.V0,
	}

	// add the object tagged fields to the global tagged fields map
	iapi.ObjectTaggedFields[versionObj] = iapi.MetricsInstanceTaggedFields[iapi.TagNameValidate]

	// add the object tagged fields to the rest API version
	api.AddRestApiVersion(versionObj)
}