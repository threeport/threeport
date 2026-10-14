package v0_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// swagInvocation matches the command that regenerates the swagger docs,
// however it is spelled.
var swagInvocation = regexp.MustCompile(`"swag",`)

// TestOneTargetRegeneratesTheSwaggerDocs covers how many ways there are to
// regenerate the committed docs, and which binary does it.
//
// Two targets did: the hand-written GenerateDocs, and GenerateSwaggerDocs
// emitted by the SDK generator. A second target named as though it were the
// authoritative one is not a harmless duplicate - whichever is reached for has
// to agree with the committed output, and the generated one resolved swag from
// $PATH rather than the version go.mod pins. Whether that agrees is a property
// of the developer's machine, so a disagreement reads as the doc pipeline being
// broken rather than as the wrong binary having run.
//
// TestSwaggerDocsIncludeAPIFields checks what the docs contain; this checks
// there is one way to produce them.
func TestOneTargetRegeneratesTheSwaggerDocs(t *testing.T) {
	t.Chdir(moduleRoot(t))

	magefiles, err := filepath.Glob(filepath.Join("magefiles", "*.go"))
	require.NoError(t, err)
	require.NotEmpty(t, magefiles)

	var invocations []string
	for _, path := range magefiles {
		body, err := os.ReadFile(path)
		require.NoError(t, err)
		for range swagInvocation.FindAllString(string(body), -1) {
			invocations = append(invocations, path)
		}
		// the pinned version is reached through the go.mod tool directive;
		// a bare command is whatever the developer happens to have installed
		assert.NotContains(
			t, string(body), "exec.Command(\n\t\t\"swag\",",
			"%s resolves swag from $PATH instead of the version go.mod pins", path,
		)
	}

	assert.Len(
		t, invocations, 1,
		"the swagger docs are regenerated one way, by %s", strings.Join(invocations, " and "),
	)
}
