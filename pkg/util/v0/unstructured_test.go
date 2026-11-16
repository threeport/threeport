package v0

import (
	"strings"
	"testing"

	"gorm.io/datatypes"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestNestedInt64OrFloat64(t *testing.T) {
	t.Run("int64", func(t *testing.T) {
		m := map[string]any{"spec": map[string]any{"port": int64(8080)}}
		got, found, err := NestedInt64OrFloat64(m, "spec", "port")
		if err != nil || !found || got != 8080 {
			t.Fatalf("got=(%d,%v,%v), want (8080,true,nil)", got, found, err)
		}
	})

	t.Run("float64 fallback", func(t *testing.T) {
		m := map[string]any{"spec": map[string]any{"port": float64(8080)}}
		got, found, err := NestedInt64OrFloat64(m, "spec", "port")
		if err != nil || !found || got != 8080 {
			t.Fatalf("got=(%d,%v,%v), want (8080,true,nil)", got, found, err)
		}
	})

	t.Run("wrong type returns error", func(t *testing.T) {
		m := map[string]any{"spec": map[string]any{"port": "nope"}}
		_, _, err := NestedInt64OrFloat64(m, "spec", "port")
		if err == nil {
			t.Fatalf("expected error")
		}
	})
}

func TestUnstructured_JSONRoundtripAndRemove(t *testing.T) {
	u1 := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name": "a",
		},
	}}
	u2 := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name": "b",
		},
	}}

	t.Run("UnstructuredToDatatypesJson and back", func(t *testing.T) {
		j, err := UnstructuredToDatatypesJson(u1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out, err := DataTypesJsonToUnstructured(&j)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.GetKind() != "ConfigMap" || out.GetName() != "a" {
			t.Fatalf("got kind/name=%s/%s, want ConfigMap/a", out.GetKind(), out.GetName())
		}
	})

	t.Run("UnstructuredListToDatatypesJsonSlice and back", func(t *testing.T) {
		slice, err := UnstructuredListToDatatypesJsonSlice([]*unstructured.Unstructured{u1, u2})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		outs, err := DataTypesJsonSliceToUnstructuredList(slice)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(outs) != 2 || outs[0].GetName() != "a" || outs[1].GetName() != "b" {
			t.Fatalf("unexpected output names: %v, %v", outs[0].GetName(), outs[1].GetName())
		}
	})

	t.Run("RemoveDataTypesJsonFromDataTypesJsonSlice removes matching item", func(t *testing.T) {
		j1, _ := UnstructuredToDatatypesJson(u1)
		j2, _ := UnstructuredToDatatypesJson(u2)
		instances := datatypes.JSONSlice[datatypes.JSON]{j1, j2}

		if err := RemoveDataTypesJsonFromDataTypesJsonSlice("a", "ConfigMap", &instances); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(instances) != 1 {
			t.Fatalf("len=%d, want 1", len(instances))
		}
		out, err := DataTypesJsonToUnstructured(&instances[0])
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.GetName() != "b" {
			t.Fatalf("remaining=%q, want %q", out.GetName(), "b")
		}
	})
}

func TestUnstructuredToYaml(t *testing.T) {
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name": "a",
		},
	}}
	y, err := UnstructuredToYaml(u)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(y, "kind: ConfigMap") || !strings.Contains(y, "name: a") {
		t.Fatalf("unexpected yaml: %q", y)
	}
}

// TestSetIndexedField writes through a list and rejects a missing index.
func TestSetIndexedField(t *testing.T) {
	// write the service name on the first rule
	object := map[string]interface{}{
		"spec": map[string]interface{}{
			"rules": []interface{}{
				map[string]interface{}{"host": "app.example.com"},
			},
		},
	}
	if err := SetIndexedField(object, "web", "spec", "rules", 0, "host"); err != nil {
		t.Fatal(err)
	}
	rules := object["spec"].(map[string]interface{})["rules"].([]interface{})
	if rules[0].(map[string]interface{})["host"] != "web" {
		t.Fatalf("host %#v", rules[0])
	}
	if err := SetIndexedField(object, "x", "spec", "rules", 1, "host"); err == nil {
		t.Fatal("expected an out of range index to fail")
	}
}

// TestMergeNamedEntries keeps a stored port and adds the new one.
func TestMergeNamedEntries(t *testing.T) {
	// keep a stored port and add the new one
	current := []interface{}{
		map[string]interface{}{"name": "tcp-5432", "port": int64(5432)},
	}
	wanted := []interface{}{
		map[string]interface{}{"name": "tcp-6379", "port": int64(6379)},
	}
	merged := MergeNamedEntries(current, wanted)
	if len(merged) != 2 {
		t.Fatalf("got %d entries, want 2", len(merged))
	}
	if merged[0].(map[string]interface{})["name"] != "tcp-5432" {
		t.Fatalf("stored entry was dropped: %#v", merged[0])
	}
	if merged[1].(map[string]interface{})["name"] != "tcp-6379" {
		t.Fatalf("new entry was not added: %#v", merged[1])
	}
}

// TestFieldAt reads a map key and a list index, and rejects a bad path.
func TestFieldAt(t *testing.T) {
	object := map[string]interface{}{
		"spec": map[string]interface{}{
			"host": "app.example.com",
			"rules": []interface{}{
				map[string]interface{}{"host": "a.example.com"},
			},
		},
	}

	// read a map key
	got, err := FieldAt(object, "spec", "host")
	if err != nil {
		t.Fatalf("read map key: %v", err)
	}
	if got != "app.example.com" {
		t.Fatalf("map key = %v", got)
	}

	// read a list index
	got, err = FieldAt(object, "spec", "rules", 0, "host")
	if err != nil {
		t.Fatalf("read list index: %v", err)
	}
	if got != "a.example.com" {
		t.Fatalf("list index = %v", got)
	}

	// reject a missing key
	if _, err = FieldAt(object, "spec", "missing"); err == nil {
		t.Fatal("missing key returned a value")
	}

	// reject an empty path
	if _, err = FieldAt(object); err == nil {
		t.Fatal("empty path returned a value")
	}

	// reject a path entry that is not a string or an int
	if _, err = FieldAt(object, "spec", 1.5); err == nil {
		t.Fatal("float path entry returned a value")
	}

	// reject a list index on a map
	if _, err = FieldAt(object, "spec", 0); err == nil {
		t.Fatal("index on a map returned a value")
	}
}

// TestRemoveIndexedField deletes a map key through a list index.
func TestRemoveIndexedField(t *testing.T) {
	object := map[string]interface{}{
		"spec": map[string]interface{}{
			"rules": []interface{}{
				map[string]interface{}{"host": "a.example.com"},
			},
		},
	}

	// delete a map key reached through a list index
	if err := RemoveIndexedField(object, "spec", "rules", 0, "host"); err != nil {
		t.Fatalf("delete map key: %v", err)
	}
	if _, err := FieldAt(object, "spec", "rules", 0, "host"); err == nil {
		t.Fatal("deleted key is still present")
	}

	// reject a delete of a list entry
	if err := RemoveIndexedField(object, "spec", "rules", 0); err == nil {
		t.Fatal("deleted a list entry")
	}

	// reject an index past the end of the list
	if err := RemoveIndexedField(object, "spec", "rules", 3, "host"); err == nil {
		t.Fatal("out of range index succeeded")
	}
}

// TestUnstructuredFromYaml decodes one YAML document.
func TestUnstructuredFromYaml(t *testing.T) {
	asset := []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: demo\n")

	// decode one document
	object, err := UnstructuredFromYaml(asset)
	if err != nil {
		t.Fatalf("decode yaml: %v", err)
	}
	if object.GetKind() != "ConfigMap" || object.GetName() != "demo" {
		t.Fatalf("decoded %s/%s", object.GetKind(), object.GetName())
	}
}

// TestMergeNamedEntriesReplacesSameName keeps one entry when the names match.
func TestMergeNamedEntriesReplacesSameName(t *testing.T) {
	current := []interface{}{
		map[string]interface{}{"name": "tcp-5432", "port": int64(5432)},
	}
	wanted := []interface{}{
		map[string]interface{}{"name": "tcp-5432", "port": int64(5433)},
	}

	// keep one entry for the matching name
	merged := MergeNamedEntries(current, wanted)
	if len(merged) != 1 {
		t.Fatalf("merged list length %d, want 1", len(merged))
	}
	entry, ok := merged[0].(map[string]interface{})
	if !ok {
		t.Fatalf("merged entry type %T", merged[0])
	}
	port, found, err := NestedInt64OrFloat64(entry, "port")
	if err != nil || !found || port != 5433 {
		t.Fatalf("replaced port = %d found=%v err=%v", port, found, err)
	}
}
