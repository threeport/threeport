package controller

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestControllerTemplateDoesNotLogTheConnectionString guards the template that
// stamps every controller's main.
//
// The connection string interpolates the broker user and password, and a
// controller's container logs are readable by anyone who can read pods in the
// control plane namespace, so logging it hands the broker credentials to them.
// The template is the only place to check: the fix has to hold for the twelve
// generated mains and for any module built with `threeport-sdk create`.
//
// The check is a count rather than a search for a log call. natsConn belongs in
// exactly two places - the assignment that builds it and the Connect that uses
// it - so a third use is it reaching something else, whatever that turns out to
// be.
func TestControllerTemplateDoesNotLogTheConnectionString(t *testing.T) {
	template, err := os.ReadFile("main.go")
	require.NoError(t, err)

	source := string(template)

	require.Contains(
		t, source, `Lit("nats://%s:%s@%s:%s")`,
		"the connection string is still what Connect is given; this test is about where else it goes",
	)
	assert.Equal(
		t, 2, strings.Count(source, `Id("natsConn")`),
		"natsConn belongs in the assignment and the Connect call only - it carries the broker password",
	)
	assert.Contains(
		t, source, `Id("natsEndpoint")`,
		"the redacted host:port value the log calls should use is missing",
	)
}

// TestGeneratedControllersDoNotLogTheConnectionString checks the output as
// well as the template, so a controller whose main was written before the fix
// and never regenerated is caught too.
func TestGeneratedControllersDoNotLogTheConnectionString(t *testing.T) {
	// the repository root, from this package's directory
	root := filepath.Join("..", "..", "..", "..", "..", "..")

	var offenders []string
	require.NoError(t, filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, "main_gen.go") {
			return err
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(source), `"NATSConnection", natsConn`) {
			offenders = append(offenders, path)
		}

		return nil
	}))

	assert.Empty(t, offenders, "these log the broker credentials; regenerate them")
}
