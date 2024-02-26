package sdk

import (
	"path/filepath"
	"strings"
	"unicode"
)

// TypeAbbrev returns a lowercase initialism of an object type,
// e.g. the object "ThisImportantThing" is abbreviated as "tit".
func TypeAbbrev(stn string) string {
	var abbrev string
	for _, r := range stn {
		if unicode.IsUpper(r) {
			abbrev += strings.ToLower(string(r))
		}
	}

	return abbrev
}

// FilenameSansExt returns the filename without the extension,
// e.g. the filename "dog_breakfast.go" is returned as "dog_breakfast".
func FilenameSansExt(filename string) string {
	return strings.TrimSuffix(filename, filepath.Ext(filename))
}
