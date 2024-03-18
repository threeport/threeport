package sdk

// GetLatestObjectVersion returns the version of the object
// based on the object name
func GetLatestObjectVersion(obj string) string {
	switch obj {
	case "AttachedObjectReference":
		return "v1"
	case "WorkloadInstance":
		return "v1"
	default:
		return "v0"
	}
}
