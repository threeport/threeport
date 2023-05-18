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

// Translate performs translation of object v if needed
func Translate(tagName string, v reflect.Value, tag reflect.StructTag) {
	if !v.CanSet() {
		return
	}
	val := tag.Get(TagNameValidate)
	if val == "" {
		return
	}
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
