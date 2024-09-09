package util

// GetDefaultObjectVersion returns the default version of the object
// based on the object name.
func GetDefaultObjectVersion(obj string) string {
	switch obj {
	case "AttachedObjectReference":
		return "v1"
	case "WorkloadInstance":
		return "v1"
	default:
		return "v0"
	}
}
