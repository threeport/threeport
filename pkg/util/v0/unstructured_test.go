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

