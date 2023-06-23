package util

// AddStringToSliceIfNotExists adds a string to a slice of strings if it does
// not currently exist in that slice.
func AddStringToSliceIfNotExists(stringSlice []string, newString string) []string {
	stringMap := make(map[string]bool)
	for _, str := range stringSlice {
		stringMap[str] = true
	}

	if stringMap[newString] {
		// string already in slice
		return stringSlice
	}

	stringSlice = append(stringSlice, newString)

	return stringSlice
}
