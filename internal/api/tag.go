package api

import "reflect"

const (
	TagNameValidate = "validate"
)

type FieldsByTag struct {
	TagName              string
	Required             []string
	Optional             []string
	OptionalAssociations []string
}

type VersionObject struct {
	Version string `json:"Version" validate:"required"`
	Object  string `json:"Object" validate:"required"`
}

var ObjectTaggedFields = make(map[VersionObject]*FieldsByTag)

//var (
//	ObjectTaggedFields                     = make(map[VersionObject]*FieldsByTag)
//	AccountTaggedFields                    = make(map[string]*FieldsByTag)
//	BlockTaggedFields                      = make(map[string]*FieldsByTag)
//	CoincoverOrderTaggedFields             = make(map[string]*FieldsByTag)
//	CompanyTaggedFields                    = make(map[string]*FieldsByTag)
//	NetworkTaggedFields                    = make(map[string]*FieldsByTag)
//	NodeTaggedFields                       = make(map[string]*FieldsByTag)
//	PoolTaggedFields                       = make(map[string]*FieldsByTag)
//	ShareTaggedFields                      = make(map[string]*FieldsByTag)
//	TokenTaggedFields                      = make(map[string]*FieldsByTag)
//	TransactionTaggedFields                = make(map[string]*FieldsByTag)
//	TransferTaggedFields                   = make(map[string]*FieldsByTag)
//	UserTaggedFields                       = make(map[string]*FieldsByTag)
//	UserIDPTaggedFields                    = make(map[string]*FieldsByTag)
//	WorkloadClusterTaggedFields            = make(map[string]*FieldsByTag)
//	WorkloadDefinitionTaggedFields         = make(map[string]*FieldsByTag)
//	WorkloadResourceDefinitionTaggedFields = make(map[string]*FieldsByTag)
//	WorkloadInstanceTaggedFields           = make(map[string]*FieldsByTag)
//	WorkloadServiceDependencyTaggedFields  = make(map[string]*FieldsByTag)
//)

// Translate performs translation of object v if needed
func Translate(tagName string, v reflect.Value, tag reflect.StructTag) {
	if !v.CanSet() {
		// unexported fields cannot be set
		//fmt.Printf("Skipping %q %s because it cannot be set.\n", v.String(), tag)
		return
	}
	val := tag.Get(TagNameValidate)
	if val == "" {
		return
	}
	// modify values if needed
	//v.SetString(fmt.Sprintf("value %q translated with %s", v.String(), val))
}

// ParseStruct parses structure's fields into respective Required, Optional and OptionalAssociations arrays
func ParseStruct(
	tagName string,
	v reflect.Value,
	tag reflect.StructTag,
	fn func(string, reflect.Value, reflect.StructTag),
	tf map[string]*FieldsByTag,
) {
	v = reflect.Indirect(v)

	switch v.Kind() {
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			switch t.Field(i).Tag.Get(tagName) {
			case REQUIRED:
				tf[tagName].Required = append(tf[tagName].Required, t.Field(i).Name)
			case OPTIONAL:
				tf[tagName].Optional = append(tf[tagName].Optional, t.Field(i).Name)
			case OPTIONAL_ASSOCIATION:
				tf[tagName].OptionalAssociations = append(tf[tagName].OptionalAssociations, t.Field(i).Name)
			}
			ParseStruct(tagName, v.Field(i), t.Field(i).Tag, fn, tf)
		}
	case reflect.Slice, reflect.Array:
		if v.Type().Elem().Kind() == reflect.String {
			for i := 0; i < v.Len(); i++ {
				ParseStruct(tagName, v.Index(i), tag, fn, tf)
			}
		}
	case reflect.String:
		fn(tagName, v, tag)
	}
}
