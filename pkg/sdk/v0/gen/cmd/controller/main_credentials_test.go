package controller

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestControllerTemplateKeepsCredentialsOutOfTheURL guards the template that
// stamps every controller's main.
//
// A controller's container logs are readable by anyone who can read pods in
// the control plane namespace, so the broker credentials must never be part of
// a string a log line or an error could carry.  The template passes them in
// the message broker config, apart from the URL, and logs only the URL.  The
// template is the only place to check: the fix has to hold for every
// generated main and for any module built with `threeport-sdk create`.
func TestControllerTemplateKeepsCredentialsOutOfTheURL(t *testing.T) {
	template, err := os.ReadFile("main.go")
	require.NoError(t, err)

	source := string(template)

	assert.NotContains(t, source, `@%s:%s`, "a connection string interpolates the broker credentials")
	assert.Contains(t, source, `Id("Password"): Op("*").Id("msgBrokerPassword")`, "the password is not passed in the broker config")
	assert.Contains(t, source, `Id("CAFile"):   Op("*").Id("msgBrokerCAFile")`, "the broker's CA is not passed, so the connection is not over TLS")
	assert.Contains(
		t, source, `Id("natsEndpoint").Op(":=").Id("brokerConfig").Dot("URL").Call()`,
		"what the log calls use is not the credential-free URL",
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
		if strings.Contains(string(source), `"NATSConnection", natsConn`) || strings.Contains(string(source), `nats://%s:%s@`) {
			offenders = append(offenders, path)
		}

		return nil
	}))

	assert.Empty(t, offenders, "these log the broker credentials; regenerate them")
}
