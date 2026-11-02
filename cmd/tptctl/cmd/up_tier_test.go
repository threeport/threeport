package cmd

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpResolvesItsTierThroughTheCli covers the link the CLI's tests cannot
// see: tptctl up takes its tier, and the notice printed when it defaults one,
// from cli.ResolveControlPlaneTier. Running the command itself would provision
// infrastructure.
func TestUpResolvesItsTierThroughTheCli(t *testing.T) {
	src, err := os.ReadFile("up.go")
	require.NoError(t, err)

	assert.Contains(t, string(src),
		"cliArgs.Tier = cli.ResolveControlPlaneTier(cliArgs.InfraProvider, cliArgs.Tier)")
}
