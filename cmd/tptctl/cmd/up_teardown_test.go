package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAFailedInstallTearsDownByDefault covers what happens to half-created
// cloud infrastructure.
//
// The flag defaulted to false, so a failure partway through `tptctl up` left
// whatever had been created - a VPC, an IAM role, a cluster - running and
// billing, and surfaced as an error that did not say so. Cleaning up was the
// operator's to remember, against resources they could not see from the
// output.
//
// The opt-out is still there for the case the default is wrong for:
// --teardown-on-failure=false keeps the partial state for someone who wants
// to look at why it failed.
func TestAFailedInstallTearsDownByDefault(t *testing.T) {
	flag := UpCmd.Flags().Lookup("teardown-on-failure")
	require.NotNil(t, flag, "the flag has to exist for its default to mean anything")

	assert.Equal(
		t, "true", flag.DefValue,
		"an install that fails should not leave cloud resources behind unless asked to",
	)
	assert.Contains(
		t, flag.Usage, "false",
		"the help has to say how to keep the partial state, now that keeping it is the exception",
	)
}
