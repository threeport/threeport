package util

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dave/jennifer/jen"
)

// render returns the bytes a jen.Code writes, trimmed of leading/trailing
// whitespace. jen's Render normalizes surrounding whitespace based on
// formatting context (a leading tab from indentation, etc.) which is
// orthogonal to what the Tag helper itself produces. Trimming keeps the
// tests focused on tag content.
func render(t *testing.T, c jen.Code) string {
	t.Helper()
	var buf bytes.Buffer
	if err := jen.Empty().Add(c).Render(&buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	return strings.TrimSpace(buf.String())
}

// TestTag_OrderPreserved is the core contract: keys are emitted in the
// order the caller supplied, NOT alphabetically. jen's built-in .Tag()
// would sort to gorm/json/validate; util.Tag must preserve the threeport
// json/validate/gorm convention.
func TestTag_OrderPreserved(t *testing.T) {
	got := render(t, Tag(
		[2]string{"json", ",omitempty"},
		[2]string{"validate", "required"},
		[2]string{"gorm", "not null"},
	))
	want := "`json:\",omitempty\" validate:\"required\" gorm:\"not null\"`"
	if got != want {
		t.Errorf("\nwant: %s\ngot:  %s", want, got)
	}
}

// TestTag_NonAlphabeticalOrder uses a reversed input ordering to prove
// the helper isn't accidentally sorting. If jen's alphabetical sort were
// in effect, this test's expected output would differ from the input order.
func TestTag_NonAlphabeticalOrder(t *testing.T) {
	got := render(t, Tag(
		[2]string{"validate", "required"},
		[2]string{"json", ",omitempty"},
	))
	want := "`validate:\"required\" json:\",omitempty\"`"
	if got != want {
		t.Errorf("\nwant: %s\ngot:  %s", want, got)
	}
}

// TestTag_SinglePair covers the degenerate one-pair input.
func TestTag_SinglePair(t *testing.T) {
	got := render(t, Tag([2]string{"json", ",omitempty"}))
	want := "`json:\",omitempty\"`"
	if got != want {
		t.Errorf("\nwant: %s\ngot:  %s", want, got)
	}
}

// TestTag_NoPairs covers Tag() with no arguments — emits empty backticks.
func TestTag_NoPairs(t *testing.T) {
	got := render(t, Tag())
	want := "``"
	if got != want {
		t.Errorf("\nwant: %s\ngot:  %s", want, got)
	}
}

// TestTag_EmptyValue covers a tag value of "", which is legal (e.g. the
// json default-name form `json:",omitempty"` is itself an empty-name tag).
func TestTag_EmptyValue(t *testing.T) {
	got := render(t, Tag([2]string{"json", ""}))
	want := "`json:\"\"`"
	if got != want {
		t.Errorf("\nwant: %s\ngot:  %s", want, got)
	}
}

// TestTag_BacktickFallback covers a tag value that contains a literal
// backtick. CanBackquote returns false, so the helper falls back to
// strconv.Quote, producing a double-quoted Go string literal instead of
// a backtick block. The generated Go file remains valid syntax.
func TestTag_BacktickFallback(t *testing.T) {
	got := render(t, Tag([2]string{"weird", "has`backtick"}))
	want := `"weird:\"has` + "`" + `backtick\""`
	if got != want {
		t.Errorf("\nwant: %s\ngot:  %s", want, got)
	}
}

// TestTag_InsideStructField is the integration shape: Tag is added to a
// jen.Statement representing a struct field inside an actual struct.
// Confirms the helper composes cleanly with surrounding jen code without
// extra spacing or quoting quirks. This mirrors how pkg/sdk/v0/create/
// api.go uses it at scaffold time.
func TestTag_InsideStructField(t *testing.T) {
	decl := jen.Type().Id("Foo").Struct(
		jen.Id("Name").Op("*").String().Add(Tag(
			[2]string{"json", ",omitempty"},
			[2]string{"validate", "required"},
		)),
	)
	got := render(t, decl)
	// jen.Empty().Add(...).Render() renders inside an outer indent context,
	// so the struct body lands at depth 2 rather than depth 1. The content
	// shape is what we care about; the surrounding indentation is jen's.
	want := "type Foo struct {\n\t\tName *string `json:\",omitempty\" validate:\"required\"`\n\t}"
	if got != want {
		t.Errorf("\nwant: %q\ngot:  %q", want, got)
	}
}
