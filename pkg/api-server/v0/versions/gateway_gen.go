// generated by 'threeport-sdk gen' - do not edit

package versions

import (
	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	apiserver_v0 "github.com/threeport/threeport/pkg/api-server/v0"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	"reflect"
)

// AddDomainNameDefinitionVersions adds field validation info and adds it
// to the REST API versions.
func AddDomainNameDefinitionVersions() {
	apiserver_v0.DomainNameDefinitionTaggedFields[apiserver_lib.TagNameValidate] = &apiserver_lib.FieldsByTag{
		Optional:             []string{},
		OptionalAssociations: []string{},
		Required:             []string{},
		TagName:              apiserver_lib.TagNameValidate,
	}

	// parse struct and populate the FieldsByTag object
	apiserver_lib.ParseStruct(
		apiserver_lib.TagNameValidate,
		reflect.ValueOf(new(api_v0.DomainNameDefinition)),
		"",
		apiserver_lib.Translate,
		apiserver_v0.DomainNameDefinitionTaggedFields,
	)

	// create a version object which contains the object name and versions
	versionObj := apiserver_lib.VersionObject{
		Object:  string(api_v0.ObjectTypeDomainNameDefinition),
		Version: "v0",
	}

	// add the object tagged fields to the global tagged fields map
	apiserver_lib.ObjectTaggedFields[versionObj] = apiserver_v0.DomainNameDefinitionTaggedFields[apiserver_lib.TagNameValidate]

	// add the object tagged fields to the rest API version
	apiserver_lib.AddObjectVersion(versionObj)
}

// AddDomainNameInstanceVersions adds field validation info and adds it
// to the REST API versions.
func AddDomainNameInstanceVersions() {
	apiserver_v0.DomainNameInstanceTaggedFields[apiserver_lib.TagNameValidate] = &apiserver_lib.FieldsByTag{
		Optional:             []string{},
		OptionalAssociations: []string{},
		Required:             []string{},
		TagName:              apiserver_lib.TagNameValidate,
	}

	// parse struct and populate the FieldsByTag object
	apiserver_lib.ParseStruct(
		apiserver_lib.TagNameValidate,
		reflect.ValueOf(new(api_v0.DomainNameInstance)),
		"",
		apiserver_lib.Translate,
		apiserver_v0.DomainNameInstanceTaggedFields,
	)

	// create a version object which contains the object name and versions
	versionObj := apiserver_lib.VersionObject{
		Object:  string(api_v0.ObjectTypeDomainNameInstance),
		Version: "v0",
	}

	// add the object tagged fields to the global tagged fields map
	apiserver_lib.ObjectTaggedFields[versionObj] = apiserver_v0.DomainNameInstanceTaggedFields[apiserver_lib.TagNameValidate]

	// add the object tagged fields to the rest API version
	apiserver_lib.AddObjectVersion(versionObj)
}

// AddGatewayDefinitionVersions adds field validation info and adds it
// to the REST API versions.
func AddGatewayDefinitionVersions() {
	apiserver_v0.GatewayDefinitionTaggedFields[apiserver_lib.TagNameValidate] = &apiserver_lib.FieldsByTag{
		Optional:             []string{},
		OptionalAssociations: []string{},
		Required:             []string{},
		TagName:              apiserver_lib.TagNameValidate,
	}

	// parse struct and populate the FieldsByTag object
	apiserver_lib.ParseStruct(
		apiserver_lib.TagNameValidate,
		reflect.ValueOf(new(api_v0.GatewayDefinition)),
		"",
		apiserver_lib.Translate,
		apiserver_v0.GatewayDefinitionTaggedFields,
	)

	// create a version object which contains the object name and versions
	versionObj := apiserver_lib.VersionObject{
		Object:  string(api_v0.ObjectTypeGatewayDefinition),
		Version: "v0",
	}

	// add the object tagged fields to the global tagged fields map
	apiserver_lib.ObjectTaggedFields[versionObj] = apiserver_v0.GatewayDefinitionTaggedFields[apiserver_lib.TagNameValidate]

	// add the object tagged fields to the rest API version
	apiserver_lib.AddObjectVersion(versionObj)
}

// AddGatewayHttpPortVersions adds field validation info and adds it
// to the REST API versions.
func AddGatewayHttpPortVersions() {
	apiserver_v0.GatewayHttpPortTaggedFields[apiserver_lib.TagNameValidate] = &apiserver_lib.FieldsByTag{
		Optional:             []string{},
		OptionalAssociations: []string{},
		Required:             []string{},
		TagName:              apiserver_lib.TagNameValidate,
	}

	// parse struct and populate the FieldsByTag object
	apiserver_lib.ParseStruct(
		apiserver_lib.TagNameValidate,
		reflect.ValueOf(new(api_v0.GatewayHttpPort)),
		"",
		apiserver_lib.Translate,
		apiserver_v0.GatewayHttpPortTaggedFields,
	)

	// create a version object which contains the object name and versions
	versionObj := apiserver_lib.VersionObject{
		Object:  string(api_v0.ObjectTypeGatewayHttpPort),
		Version: "v0",
	}

	// add the object tagged fields to the global tagged fields map
	apiserver_lib.ObjectTaggedFields[versionObj] = apiserver_v0.GatewayHttpPortTaggedFields[apiserver_lib.TagNameValidate]

	// add the object tagged fields to the rest API version
	apiserver_lib.AddObjectVersion(versionObj)
}

// AddGatewayInstanceVersions adds field validation info and adds it
// to the REST API versions.
func AddGatewayInstanceVersions() {
	apiserver_v0.GatewayInstanceTaggedFields[apiserver_lib.TagNameValidate] = &apiserver_lib.FieldsByTag{
		Optional:             []string{},
		OptionalAssociations: []string{},
		Required:             []string{},
		TagName:              apiserver_lib.TagNameValidate,
	}

	// parse struct and populate the FieldsByTag object
	apiserver_lib.ParseStruct(
		apiserver_lib.TagNameValidate,
		reflect.ValueOf(new(api_v0.GatewayInstance)),
		"",
		apiserver_lib.Translate,
		apiserver_v0.GatewayInstanceTaggedFields,
	)

	// create a version object which contains the object name and versions
	versionObj := apiserver_lib.VersionObject{
		Object:  string(api_v0.ObjectTypeGatewayInstance),
		Version: "v0",
	}

	// add the object tagged fields to the global tagged fields map
	apiserver_lib.ObjectTaggedFields[versionObj] = apiserver_v0.GatewayInstanceTaggedFields[apiserver_lib.TagNameValidate]

	// add the object tagged fields to the rest API version
	apiserver_lib.AddObjectVersion(versionObj)
}

// AddGatewayTcpPortVersions adds field validation info and adds it
// to the REST API versions.
func AddGatewayTcpPortVersions() {
	apiserver_v0.GatewayTcpPortTaggedFields[apiserver_lib.TagNameValidate] = &apiserver_lib.FieldsByTag{
		Optional:             []string{},
		OptionalAssociations: []string{},
		Required:             []string{},
		TagName:              apiserver_lib.TagNameValidate,
	}

	// parse struct and populate the FieldsByTag object
	apiserver_lib.ParseStruct(
		apiserver_lib.TagNameValidate,
		reflect.ValueOf(new(api_v0.GatewayTcpPort)),
		"",
		apiserver_lib.Translate,
		apiserver_v0.GatewayTcpPortTaggedFields,
	)

	// create a version object which contains the object name and versions
	versionObj := apiserver_lib.VersionObject{
		Object:  string(api_v0.ObjectTypeGatewayTcpPort),
		Version: "v0",
	}

	// add the object tagged fields to the global tagged fields map
	apiserver_lib.ObjectTaggedFields[versionObj] = apiserver_v0.GatewayTcpPortTaggedFields[apiserver_lib.TagNameValidate]

	// add the object tagged fields to the rest API version
	apiserver_lib.AddObjectVersion(versionObj)
}
