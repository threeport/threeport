package v0

import (
	"reflect"
	"testing"
)

func TestIsNonNilPtr(t *testing.T) {
	var ps *string
	s := "x"
	if IsNonNilPtr(reflect.ValueOf(ps)) {
		t.Fatalf("nil ptr should be false")
	}
	if !IsNonNilPtr(reflect.ValueOf(&s)) {
		t.Fatalf("non-nil ptr should be true")
	}
	if IsNonNilPtr(reflect.ValueOf(s)) {
		t.Fatalf("non-ptr should be false")
	}
}

func TestGetPtrValue(t *testing.T) {
	t.Run("nil ptr returns empty nil", func(t *testing.T) {
		var ps *string
		got, err := GetPtrValue(reflect.ValueOf(ps))
		if err != nil || got != "" {
			t.Fatalf("got=(%q,%v), want=(\"\",nil)", got, err)
		}
	})

	t.Run("string ptr returns value", func(t *testing.T) {
		s := "hello"
		got, err := GetPtrValue(reflect.ValueOf(&s))
		if err != nil || got != "hello" {
			t.Fatalf("got=(%q,%v), want=(%q,nil)", got, err, "hello")
		}
	})

	t.Run("non-string ptr returns error", func(t *testing.T) {
		i := 1
		_, err := GetPtrValue(reflect.ValueOf(&i))
		if err == nil {
			t.Fatalf("expected error")
		}
	})
}

func TestGetObjectFieldValue(t *testing.T) {
	type S struct {
		Name string
		PS   *string
		PI   *int
	}

	val := "v"
	obj := S{Name: "n", PS: &val}

	t.Run("struct field value", func(t *testing.T) {
		v, err := GetObjectFieldValue(obj, "Name")
		if err != nil || v.Kind() != reflect.String || v.String() != "n" {
			t.Fatalf("got=(%v,%v), want string \"n\"", v, err)
		}
	})

	t.Run("ptr-to-struct works", func(t *testing.T) {
		v, err := GetObjectFieldValue(&obj, "Name")
		if err != nil || v.String() != "n" {
			t.Fatalf("got=(%v,%v), want string \"n\"", v, err)
		}
	})

	t.Run("nil pointer field returns no value", func(t *testing.T) {
		v, err := GetObjectFieldValue(obj, "PI")
		if err != nil || v.Kind() != reflect.String || v.String() != "no value" {
			t.Fatalf("got=(%v,%v), want string \"no value\"", v, err)
		}
	})

	t.Run("non-nil pointer field returns elem", func(t *testing.T) {
		v, err := GetObjectFieldValue(obj, "PS")
		if err != nil || v.Kind() != reflect.String || v.String() != "v" {
			t.Fatalf("got=(%v,%v), want string \"v\"", v, err)
		}
	})

	t.Run("missing field errors", func(t *testing.T) {
		if _, err := GetObjectFieldValue(obj, "Nope"); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("non-struct errors", func(t *testing.T) {
		if _, err := GetObjectFieldValue(123, "X"); err == nil {
			t.Fatalf("expected error")
		}
	})
}

