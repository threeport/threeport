package v0_test

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSwaggerDocsIncludeEksFields checks that the published EKS schemas keep
// the fields from the Go types.
func TestSwaggerDocsIncludeEksFields(t *testing.T) {
	body, err := os.ReadFile("docs/swagger.yaml")
	require.NoError(t, err)

	definition := swaggerSection(t, string(body), "v0.AwsEksKubernetesRuntimeDefinition")
	require.Contains(t, definition, "ZoneCount:")
	require.Contains(t, definition, "DefaultNodeGroupInstanceType:")

	instance := swaggerSection(t, string(body), "v0.AwsEksKubernetesRuntimeInstance")
	require.Contains(t, instance, "AwsProviderID:")
	require.Contains(t, instance, "ResourceInventory:")
}

// swaggerSection returns the YAML block for one definition name.
func swaggerSection(t *testing.T, body string, name string) string {
	t.Helper()
	marker := "  " + name + ":\n"
	start := strings.Index(body, marker)
	require.NotEqual(t, -1, start)
	rest := body[start+len(marker):]
	next := strings.Index(rest, "\n  v0.")
	if next == -1 {
		return rest
	}
	return rest[:next]
}
