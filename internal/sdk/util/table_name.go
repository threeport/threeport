package util

import (
	"regexp"
	"strings"
)

// TableName takes any string and turns it into a valid database table name.
func TableName(input string) string {
	// convert to lowercase
	input = strings.ToLower(input)

	// replace any non-alphanumeric character (except underscores) with an underscore
	re := regexp.MustCompile(`[^a-z0-9_]`)
	output := re.ReplaceAllString(input, "_")

	// remove any leading or trailing underscores
	output = strings.Trim(output, "_")

	// ensure the length of the table name does not exceed 63 characters
	if len(output) > 63 {
		output = output[:63]
	}

	return output
}
