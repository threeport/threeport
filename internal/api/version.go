package api

import (
	_ "embed"
	"strings"
)

// Version is a constant variable containing the version
//
//go:embed version.txt
var Version string

// GetVersion Returns REST API Version
func GetVersion() string {
	return strings.TrimSuffix(Version, "\n")
}

// RESTAPIVersion provides the version of the REST API binary.
type RESTAPIVersion struct {
	Version string `json:"Version" validate:"required"`
}
