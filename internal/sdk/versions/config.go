package versions

// GlobalVersionConfig contains all API versions for which code is being
// generated.
type GlobalVersionConfig struct {
	Versions []VersionConfig
}

// VersionConfig is the configuration for a given version.
type VersionConfig struct {
	VersionName       string
	RouteNames        []string
	DatabaseInitNames []string
	ReconciledNames   []string
}

// getQualifiedPath returns the qualified path for the client library code
// based on the API version.
func (vc VersionConfig) getQualifiedPath() string {
	switch vc.VersionName {
	case "v0":
		return ""
	default:
		return "github.com/threeport/threeport/pkg/api-server/v0"
	}
}
