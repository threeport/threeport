package v0

import (
	"net/url"
	"strings"
	"testing"
)

func TestStringUtils(t *testing.T) {
	t.Run("CreateQueryStringFromMap parses to same map", func(t *testing.T) {
		in := map[string]string{"a": "1", "b": "two"}
		q, err := url.ParseQuery(CreateQueryStringFromMap(in))
		if err != nil {
			t.Fatalf("ParseQuery error: %v", err)
		}
		if q.Get("a") != "1" || q.Get("b") != "two" {
			t.Fatalf("parsed=%v, want a=1 b=two", q)
		}
	})

	t.Run("StringSliceContains respects caseSensitive", func(t *testing.T) {
		sl := []string{"Foo"}
		if !StringSliceContains(sl, "Foo", true) || StringSliceContains(sl, "foo", true) {
			t.Fatalf("caseSensitive behavior unexpected")
		}
		if !StringSliceContains(sl, "foo", false) {
			t.Fatalf("case-insensitive match expected")
		}
	})

	t.Run("RandomAlphaString length+charset", func(t *testing.T) {
		s := RandomAlphaString(32)
		if len(s) != 32 {
			t.Fatalf("len=%d, want 32", len(s))
		}
		if strings.Trim(s, alphaCharset) != "" {
			t.Fatalf("unexpected chars in %q", s)
		}
	})

	t.Run("RandomAlphaNumericString length+charset", func(t *testing.T) {
		s := RandomAlphaNumericString(32)
		if len(s) != 32 {
			t.Fatalf("len=%d, want 32", len(s))
		}
		if strings.Trim(s, alphaNumericCharset) != "" {
			t.Fatalf("unexpected chars in %q", s)
		}
	})

	t.Run("Base64Encode/Base64Decode roundtrip", func(t *testing.T) {
		in := "hello, world"
		out, err := Base64Decode(Base64Encode(in))
		if err != nil || out != in {
			t.Fatalf("roundtrip out=%q err=%v, want %q nil", out, err, in)
		}
	})

	t.Run("Base64Decode invalid returns error", func(t *testing.T) {
		if _, err := Base64Decode("not base64!!"); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("StringListContains", func(t *testing.T) {
		if !StringListContains("b", []string{"a", "b"}) || StringListContains("c", []string{"a", "b"}) {
			t.Fatalf("unexpected contains result")
		}
	})

	t.Run("StringToInterfaceList preserves values", func(t *testing.T) {
		got := StringToInterfaceList([]string{"a", "b"})
		if len(got) != 2 || got[0] != "a" || got[1] != "b" {
			t.Fatalf("got=%v, want [a b]", got)
		}
	})

	t.Run("HyphenDelimitedString formats entries", func(t *testing.T) {
		if got, want := HyphenDelimitedString([]string{"x", "y"}), "---\nx---\ny"; got != want {
			t.Fatalf("got=%q, want %q", got, want)
		}
	})

	t.Run("TypeName", func(t *testing.T) {
		if got := TypeName(1); got != "int" {
			t.Fatalf("got=%q, want int", got)
		}
	})

	t.Run("TruncateString", func(t *testing.T) {
		if got := TruncateString("hello", 10); got != "hello" {
			t.Fatalf("got=%q, want hello", got)
		}
		if got := TruncateString("hello world", 5); got != "hello..." {
			t.Fatalf("got=%q, want hello...", got)
		}
	})
}

