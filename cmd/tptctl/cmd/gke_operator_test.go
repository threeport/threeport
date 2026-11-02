package cmd

import (
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTptctlAuthenticatesToGkeAsAnOperator covers #646: tptctl is run by a
// person, who acts on a GKE cluster with their own gcloud login. A process that
// does not say so is held to the controllers' policy, which never uses a
// gcloud login, and every command that reaches a GKE cluster fails for a
// genesis control plane, whose GCP provider stores no credentials.
func TestTptctlAuthenticatesToGkeAsAnOperator(t *testing.T) {
	src, err := os.ReadFile("root.go")
	require.NoError(t, err)

	execute := regexp.MustCompile(`func Execute\(\) \{[\s\S]*?\n}`).FindString(string(src))
	require.NotEmpty(t, execute, "Execute not found")
	assert.Contains(t, execute, "kube.AuthenticateToGkeAsOperator()",
		"tptctl declares itself an operator before any command runs")
}
