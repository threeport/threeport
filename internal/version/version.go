package version

import (
	_ "embed"
	"strings"
)

// Version is a constant variable containing the version
//
//go:embed version.txt
var Version string

// GetVersion returns the version of Threeport as set in `version.txt`.
func GetVersion() string {
	return strings.TrimSuffix(Version, "\n")
}

// RESTAPIVersion provides the version of the REST API binary.
type RESTAPIVersion struct {
	Version string `json:"Version" validate:"required"`
}
