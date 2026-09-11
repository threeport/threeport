package v0

import (
	"os"
	"strings"
)

// EnvOr returns the trimmed value of the named env var, or def if it is
// unset or empty.
func EnvOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
