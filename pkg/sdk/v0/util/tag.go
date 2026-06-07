package util

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/dave/jennifer/jen"
)

// OrderedTag is a jen.Code emitter for struct tags that preserves caller-specified
// key order. jen's built-in .Tag(map[string]string) sorts keys alphabetically;
// OrderedTag renders keys in the order they were supplied so generated tags can
// follow the threeport convention (json, validate, gorm, ...).
//
// It is an alias for *jen.Statement so it satisfies jen.Code (which has
// unexported methods only types in the jen package can implement) and exposes
// jen.Statement's exported Render(w io.Writer) error.
type OrderedTag = *jen.Statement

// Tag returns a jen.Code that renders the supplied (key, value) pairs as a
// backtick-quoted struct tag in the order they were supplied. Each pair is a
// [2]string of {key, value}. The emitted output is preceded by a single space
// separator when appended to a jen.Statement, matching jen's built-in .Tag()
// behavior.
//
// The implementation builds the formatted tag string and hands it to jen.Op,
// which writes the content verbatim. This sidesteps jen.Tag's alphabetical
// key sort.
func Tag(pairs ...[2]string) jen.Code {
	var parts []string
	for _, pair := range pairs {
		parts = append(parts, fmt.Sprintf(`%s:%q`, pair[0], pair[1]))
	}
	joined := strings.Join(parts, " ")

	// wrap in backticks when safe; fall back to a double-quoted string for
	// values containing backticks or non-printable characters
	var rendered string
	if strconv.CanBackquote(joined) {
		rendered = "`" + joined + "`"
	} else {
		rendered = strconv.Quote(joined)
	}

	// leading space so the tag composes cleanly when added to a struct
	// field (matches jen's built-in .Tag() spacing behavior).
	return jen.Op(" " + rendered)
}
