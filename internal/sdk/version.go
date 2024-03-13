package sdk

// GetObjectVersion returns the version of the object
// based on the object name
func GetObjectVersion(obj string) string {
	switch obj {
	case "WorkloadInstance":
		return "v1"
	default:
		return "v0"
	}
}
