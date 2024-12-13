package util

import (
	"regexp"
	"strings"
)

// RestPath returns a REST path friendly version of a string.
func RestPath(s string) string {
	// convert to lowercase
	s = strings.ToLower(s)

	// replace spaces, dots and fwd slashes with hyphens
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, ".", "-")
	s = strings.ReplaceAll(s, "/", "-")

	// remove any non-alphanumeric characters except for hyphens
	reg, _ := regexp.Compile("[^a-z0-9-]+")
	s = reg.ReplaceAllString(s, "")

	// trim any leading or trailing hyphens
	s = strings.Trim(s, "-")

	return s
}
