package v0

import (
	"reflect"
	"testing"
)

// TestModuleApiCarriesImageColumns reads the columns Gorm stores for the
// module image fields.
func TestModuleApiCarriesImageColumns(t *testing.T) {
	// read the column each field declares
	want := map[string]string{
		"ImageNamespace": "image_namespace",
		"ImageTag":       "image_tag",
	}
	typ := reflect.TypeOf(ModuleApi{})
	for field, column := range want {
		f, ok := typ.FieldByName(field)
		if !ok {
			t.Fatalf("ModuleApi is missing %s", field)
		}
		if got := f.Tag.Get("gorm"); got != "column:"+column {
			t.Fatalf("%s gorm tag %q, want column:%s", field, got, column)
		}
	}
}
