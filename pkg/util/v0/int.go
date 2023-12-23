package v0

// IntSliceContains returns true if a slice contains a certain int.
func IntSliceContains(sl []int, value int) bool {
	for _, val := range sl {
		if val == value {
			return true
		}
	}

	return false
}
