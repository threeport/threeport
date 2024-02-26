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
