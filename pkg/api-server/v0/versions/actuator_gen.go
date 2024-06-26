// generated by 'threeport-sdk codegen' for API version boilerplate - do not edit

package versions

import (
	api "github.com/threeport/threeport/pkg/api"
	iapi "github.com/threeport/threeport/pkg/api-server/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	"reflect"
)

// AddProfileVersions adds field validation info and adds it
// to the REST API versions.
func AddProfileVersions() {
	iapi.ProfileTaggedFields[iapi.TagNameValidate] = &iapi.FieldsByTag{
		Optional:             []string{},
		OptionalAssociations: []string{},
		Required:             []string{},
		TagName:              iapi.TagNameValidate,
	}

	// parse struct and populate the FieldsByTag object
	iapi.ParseStruct(
		iapi.TagNameValidate,
		reflect.ValueOf(new(v0.Profile)),
		"",
		iapi.Translate,
		iapi.ProfileTaggedFields,
	)

	// create a version object which contains the object name and versions
	versionObj := iapi.VersionObject{
		Object:  string(v0.ObjectTypeProfile),
		Version: "v0",
	}

	// add the object tagged fields to the global tagged fields map
	iapi.ObjectTaggedFields[versionObj] = iapi.ProfileTaggedFields[iapi.TagNameValidate]

	// add the object tagged fields to the rest API version
	api.AddRestApiVersion(versionObj)
}

// AddTierVersions adds field validation info and adds it
// to the REST API versions.
func AddTierVersions() {
	iapi.TierTaggedFields[iapi.TagNameValidate] = &iapi.FieldsByTag{
		Optional:             []string{},
		OptionalAssociations: []string{},
		Required:             []string{},
		TagName:              iapi.TagNameValidate,
	}

	// parse struct and populate the FieldsByTag object
	iapi.ParseStruct(
		iapi.TagNameValidate,
		reflect.ValueOf(new(v0.Tier)),
		"",
		iapi.Translate,
		iapi.TierTaggedFields,
	)

	// create a version object which contains the object name and versions
	versionObj := iapi.VersionObject{
		Object:  string(v0.ObjectTypeTier),
		Version: "v0",
	}

	// add the object tagged fields to the global tagged fields map
	iapi.ObjectTaggedFields[versionObj] = iapi.TierTaggedFields[iapi.TagNameValidate]

	// add the object tagged fields to the rest API version
	api.AddRestApiVersion(versionObj)
}
