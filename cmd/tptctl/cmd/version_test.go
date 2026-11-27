package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "sigs.k8s.io/yaml"
)

const (
	testClientVersion = "v0.7.0-rc.6.a1b2c3d"
	testServerVersion = "v0.7.0-rc.6.e4f5a6b"
)

// versionRun is what running the version command wrote and returned.
type versionRun struct {
	stdout      string
	stderr      string
	exitCode    int
	lookupCalls int
}

// runVersionWith runs the version command with a lookup that returns server.
func runVersionWith(opts versionOptions, clientVersion string, server *serverVersionReport) versionRun {
	var stdout, stderr bytes.Buffer
	run := versionRun{}
	run.exitCode = runVersion(&stdout, &stderr, opts, clientVersion, func() *serverVersionReport {
		run.lookupCalls++
		return server
	})
	run.stdout = stdout.String()
	run.stderr = stderr.String()

	return run
}

func matchingServer() *serverVersionReport {
	return &serverVersionReport{
		ControlPlane: "genesis-00",
		APIEndpoint:  "https://api.example.com:443",
		Version:      testClientVersion,
	}
}

func mismatchedServer() *serverVersionReport {
	server := matchingServer()
	server.Version = testServerVersion

	return server
}

func unreadableServer() *serverVersionReport {
	return &serverVersionReport{
		ControlPlane: "genesis-00",
		APIEndpoint:  "https://api.example.com:443",
		Error:        "failed to execute call to threeport API: connection refused",
	}
}

func TestBuildVersionReport(t *testing.T) {
	t.Run("the same version matches", func(t *testing.T) {
		report := buildVersionReport(testClientVersion, matchingServer())
		assert.True(t, report.Match)
		assert.False(t, report.versionMismatch())
	})

	t.Run("a different version does not match", func(t *testing.T) {
		report := buildVersionReport(testClientVersion, mismatchedServer())
		assert.False(t, report.Match)
		assert.True(t, report.versionMismatch())
	})

	t.Run("whitespace around either version is ignored", func(t *testing.T) {
		server := matchingServer()
		server.Version = "  " + testClientVersion + "\n"
		report := buildVersionReport(testClientVersion+"\n", server)
		assert.True(t, report.Match)
		assert.Equal(t, testClientVersion, report.ClientVersion)
		assert.Equal(t, testClientVersion, report.Server.Version)
	})

	t.Run("versions are compared as strings and never ordered", func(t *testing.T) {
		// a newer-looking client is still a mismatch, and so is a shorter one
		for _, serverVersion := range []string{"v0.7.0-rc.5", "v0.7.0-rc.6", "v0.7.0-rc.6.a1b2c3", "V0.7.0-RC.6.A1B2C3D"} {
			server := matchingServer()
			server.Version = serverVersion
			assert.False(t, buildVersionReport(testClientVersion, server).Match, serverVersion)
		}
	})

	t.Run("an unreadable server version has no match and is not a mismatch", func(t *testing.T) {
		report := buildVersionReport(testClientVersion, unreadableServer())
		assert.False(t, report.Match)
		assert.False(t, report.versionMismatch())
	})

	t.Run("an error never matches, even with the client's version beside it", func(t *testing.T) {
		server := matchingServer()
		server.Error = "something went wrong"
		assert.False(t, buildVersionReport(testClientVersion, server).Match)
	})

	t.Run("no control plane is no server and no match", func(t *testing.T) {
		report := buildVersionReport(testClientVersion, nil)
		assert.Nil(t, report.Server)
		assert.False(t, report.Match)
		assert.False(t, report.versionMismatch())
	})

	t.Run("the report does not alias the server it was built from", func(t *testing.T) {
		server := matchingServer()
		server.Version = " " + testClientVersion
		_ = buildVersionReport(testClientVersion, server)
		assert.Equal(t, " "+testClientVersion, server.Version)
	})
}

func TestRunVersionText(t *testing.T) {
	text := versionOptions{format: "text"}

	t.Run("a matching control plane prints both versions and exits 0", func(t *testing.T) {
		run := runVersionWith(text, testClientVersion, matchingServer())

		assert.Equal(t,
			"Client Version: "+testClientVersion+"\n"+
				"Server Version: "+testClientVersion+" (control plane genesis-00)\n",
			run.stdout,
		)
		assert.Empty(t, run.stderr)
		assert.Equal(t, 0, run.exitCode)
	})

	t.Run("a mismatch prints both versions to stdout, warns on stderr and exits 0", func(t *testing.T) {
		run := runVersionWith(text, testClientVersion, mismatchedServer())

		assert.Equal(t,
			"Client Version: "+testClientVersion+"\n"+
				"Server Version: "+testServerVersion+" (control plane genesis-00)\n",
			run.stdout,
		)
		assert.Equal(t,
			"Warning: tptctl version "+testClientVersion+" does not match control plane genesis-00 version "+testServerVersion+".\n"+
				"Install a matching tptctl before installing, upgrading or changing this control plane. "+
				"See https://threeport.io/install/install-tptctl/ and docs/dev/head-builds.md.\n",
			run.stderr,
		)
		assert.Equal(t, 0, run.exitCode)
	})

	t.Run("no control plane prints none and exits 0", func(t *testing.T) {
		run := runVersionWith(text, testClientVersion, nil)

		assert.Equal(t,
			"Client Version: "+testClientVersion+"\n"+
				"Server Version: none (no control plane in the Threeport config)\n",
			run.stdout,
		)
		assert.Empty(t, run.stderr)
		assert.Equal(t, 0, run.exitCode)
	})

	t.Run("an unreadable server prints the client, the error on stderr and exits 1", func(t *testing.T) {
		run := runVersionWith(text, testClientVersion, unreadableServer())

		assert.Equal(t, "Client Version: "+testClientVersion+"\n", run.stdout)
		assert.Contains(t, run.stderr,
			"Error: failed to read the version of control plane genesis-00: failed to execute call to threeport API: connection refused")
		assert.Contains(t, run.stderr, "--client")
		assert.NotContains(t, run.stderr, "Warning")
		assert.Equal(t, 1, run.exitCode)
	})

	t.Run("an error with no control plane to name still reads sensibly", func(t *testing.T) {
		run := runVersionWith(text, testClientVersion, &serverVersionReport{Error: "failed to read config file"})

		assert.Contains(t, run.stderr, "Error: failed to read the control plane version: failed to read config file")
		assert.Equal(t, 1, run.exitCode)
	})

	t.Run("--client prints only the client version and never looks up the server", func(t *testing.T) {
		run := runVersionWith(versionOptions{format: "text", clientOnly: true}, testClientVersion, mismatchedServer())

		assert.Equal(t, "Client Version: "+testClientVersion+"\n", run.stdout)
		assert.Empty(t, run.stderr)
		assert.Equal(t, 0, run.exitCode)
		assert.Equal(t, 0, run.lookupCalls)
	})
}

func TestRunVersionJSON(t *testing.T) {
	jsonOpts := versionOptions{format: "json"}

	t.Run("a matching control plane has the documented structure", func(t *testing.T) {
		run := runVersionWith(jsonOpts, testClientVersion, matchingServer())

		assert.JSONEq(t, `{
			"clientVersion": "`+testClientVersion+`",
			"server": {
				"controlPlane": "genesis-00",
				"apiEndpoint": "https://api.example.com:443",
				"version": "`+testClientVersion+`",
				"error": ""
			},
			"match": true
		}`, run.stdout)
		assert.Empty(t, run.stderr)
		assert.Equal(t, 0, run.exitCode)
	})

	t.Run("a mismatch is match false, exits 0 and warns on stderr", func(t *testing.T) {
		run := runVersionWith(jsonOpts, testClientVersion, mismatchedServer())

		var report versionReport
		require.NoError(t, json.Unmarshal([]byte(run.stdout), &report), "stdout must be only the JSON document")
		assert.False(t, report.Match)
		assert.Equal(t, testServerVersion, report.Server.Version)
		assert.Contains(t, run.stderr, "does not match control plane genesis-00")
		assert.Equal(t, 0, run.exitCode)
	})

	t.Run("no control plane has a null server, present in the output", func(t *testing.T) {
		run := runVersionWith(jsonOpts, testClientVersion, nil)

		var fields map[string]any
		require.NoError(t, json.Unmarshal([]byte(run.stdout), &fields))
		server, present := fields["server"]
		assert.True(t, present, "server must be in the output, as null")
		assert.Nil(t, server)
		assert.Equal(t, false, fields["match"])
		assert.Equal(t, testClientVersion, fields["clientVersion"])
		assert.Equal(t, 0, run.exitCode)
	})

	t.Run("--client has a null server", func(t *testing.T) {
		run := runVersionWith(versionOptions{format: "json", clientOnly: true}, testClientVersion, matchingServer())

		var fields map[string]any
		require.NoError(t, json.Unmarshal([]byte(run.stdout), &fields))
		assert.Nil(t, fields["server"])
		assert.Equal(t, false, fields["match"])
		assert.Equal(t, 0, run.exitCode)
		assert.Equal(t, 0, run.lookupCalls)
	})

	t.Run("an unreadable server puts the error in the output and exits 1", func(t *testing.T) {
		run := runVersionWith(jsonOpts, testClientVersion, unreadableServer())

		var fields map[string]any
		require.NoError(t, json.Unmarshal([]byte(run.stdout), &fields), "stdout must be only the JSON document")
		server, ok := fields["server"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "failed to execute call to threeport API: connection refused", server["error"])
		assert.Equal(t, "", server["version"])
		assert.Equal(t, false, fields["match"])
		assert.Contains(t, run.stderr, "connection refused")
		assert.Equal(t, 1, run.exitCode)
	})
}

func TestRunVersionYAML(t *testing.T) {
	yamlOpts := versionOptions{format: "yaml"}

	// parse stdout the way a consumer would, through YAML alone
	parse := func(t *testing.T, stdout string) map[string]any {
		t.Helper()
		var fields map[string]any
		require.NoError(t, yaml.Unmarshal([]byte(stdout), &fields), "stdout must be only the YAML document")

		return fields
	}

	t.Run("a matching control plane has the same structure as the JSON", func(t *testing.T) {
		run := runVersionWith(yamlOpts, testClientVersion, matchingServer())

		fields := parse(t, run.stdout)
		assert.Equal(t, testClientVersion, fields["clientVersion"])
		assert.Equal(t, true, fields["match"])
		assert.Equal(t, map[string]any{
			"controlPlane": "genesis-00",
			"apiEndpoint":  "https://api.example.com:443",
			"version":      testClientVersion,
			"error":        "",
		}, fields["server"])
		assert.Empty(t, run.stderr)
		assert.Equal(t, 0, run.exitCode)
	})

	t.Run("a mismatch warns on stderr and exits 0", func(t *testing.T) {
		run := runVersionWith(yamlOpts, testClientVersion, mismatchedServer())

		assert.Equal(t, false, parse(t, run.stdout)["match"])
		assert.Contains(t, run.stderr, "does not match control plane genesis-00")
		assert.Equal(t, 0, run.exitCode)
	})

	t.Run("no control plane has a null server, present in the output", func(t *testing.T) {
		run := runVersionWith(yamlOpts, testClientVersion, nil)

		fields := parse(t, run.stdout)
		server, present := fields["server"]
		assert.True(t, present, "server must be in the output, as null")
		assert.Nil(t, server)
		assert.Equal(t, false, fields["match"])
		assert.Equal(t, 0, run.exitCode)
	})

	t.Run("an unreadable server puts the error in the output and exits 1", func(t *testing.T) {
		run := runVersionWith(yamlOpts, testClientVersion, unreadableServer())

		server, ok := parse(t, run.stdout)["server"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "failed to execute call to threeport API: connection refused", server["error"])
		assert.Equal(t, 1, run.exitCode)
	})
}

func TestRunVersionRefusesAnUnknownFormat(t *testing.T) {
	run := runVersionWith(versionOptions{format: "xml"}, testClientVersion, matchingServer())

	assert.Empty(t, run.stdout)
	assert.Contains(t, run.stderr, `unrecognized output format "xml"`)
	assert.Equal(t, 1, run.exitCode)
	assert.Equal(t, 0, run.lookupCalls, "the control plane must not be contacted for a format that is refused")
}

// versionConfig writes a Threeport config and returns its path.  viper holds
// the loaded config globally, so it is reset when the test ends.
func versionConfig(t *testing.T, content string) string {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)

	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

// versionServerFor serves version as the REST API's /version route and returns
// the address the way the Threeport config stores one, without a scheme.
func versionServerFor(t *testing.T, version string) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/version" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprintf(w, `{"Version":%q}`, version)
	}))
	t.Cleanup(server.Close)

	return strings.TrimPrefix(server.URL, "http://")
}

// closedAddress returns an address that nothing answers on.
func closedAddress(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.NotFoundHandler())
	address := strings.TrimPrefix(server.URL, "http://")
	server.Close()

	return address
}

func TestLookupServerVersion(t *testing.T) {
	t.Run("no config file means no control plane", func(t *testing.T) {
		viper.Reset()
		t.Cleanup(viper.Reset)
		missing := filepath.Join(t.TempDir(), "config.yaml")

		assert.Nil(t, lookupServerVersion(missing, ""))
	})

	t.Run("a named control plane with no config file is an error", func(t *testing.T) {
		viper.Reset()
		t.Cleanup(viper.Reset)
		missing := filepath.Join(t.TempDir(), "config.yaml")

		report := lookupServerVersion(missing, "genesis-00")

		require.NotNil(t, report)
		assert.Equal(t, "genesis-00", report.ControlPlane)
		assert.Contains(t, report.Error, "no Threeport config file")
	})

	t.Run("an empty config means no control plane", func(t *testing.T) {
		assert.Nil(t, lookupServerVersion(versionConfig(t, "{}\n"), ""))
	})

	t.Run("the empty current control plane that tptctl down leaves means no control plane", func(t *testing.T) {
		path := versionConfig(t, "CurrentControlPlane: \"\"\nControlPlanes:\n  - Name: \"\"\n")

		assert.Nil(t, lookupServerVersion(path, ""))
	})

	t.Run("a config that cannot be parsed is an error", func(t *testing.T) {
		path := versionConfig(t, "CurrentControlPlane: [unclosed\n")

		report := lookupServerVersion(path, "")

		require.NotNil(t, report)
		assert.NotEmpty(t, report.Error)
	})

	t.Run("the current control plane's version is read", func(t *testing.T) {
		address := versionServerFor(t, testServerVersion)
		path := versionConfig(t, fmt.Sprintf(
			"CurrentControlPlane: cp-a\nControlPlanes:\n  - Name: cp-a\n    APIServer: %s\n    AuthEnabled: false\n", address,
		))

		report := lookupServerVersion(path, "")

		require.NotNil(t, report)
		assert.Empty(t, report.Error)
		assert.Equal(t, "cp-a", report.ControlPlane)
		assert.Equal(t, testServerVersion, report.Version)
		assert.Equal(t, "http://"+address, report.APIEndpoint)
	})

	t.Run("a named control plane is used in place of the current one", func(t *testing.T) {
		addressA := versionServerFor(t, "v-from-a")
		addressB := versionServerFor(t, "v-from-b")
		path := versionConfig(t, fmt.Sprintf(
			"CurrentControlPlane: cp-a\nControlPlanes:\n"+
				"  - Name: cp-a\n    APIServer: %s\n"+
				"  - Name: cp-b\n    APIServer: %s\n", addressA, addressB,
		))

		report := lookupServerVersion(path, "cp-b")

		require.NotNil(t, report)
		assert.Equal(t, "cp-b", report.ControlPlane)
		assert.Equal(t, "v-from-b", report.Version)
	})

	t.Run("a named control plane that is not in the config is an error that lists the ones that are", func(t *testing.T) {
		path := versionConfig(t, fmt.Sprintf(
			"CurrentControlPlane: cp-a\nControlPlanes:\n  - Name: cp-a\n    APIServer: %s\n", versionServerFor(t, "v1"),
		))

		report := lookupServerVersion(path, "nope")

		require.NotNil(t, report)
		assert.Equal(t, "nope", report.ControlPlane)
		assert.Contains(t, report.Error, `"nope"`)
		assert.Contains(t, report.Error, "cp-a")
		assert.Empty(t, report.Version)
	})

	t.Run("a control plane nothing answers for is an error, with the endpoint it tried", func(t *testing.T) {
		address := closedAddress(t)
		path := versionConfig(t, fmt.Sprintf(
			"CurrentControlPlane: cp-a\nControlPlanes:\n  - Name: cp-a\n    APIServer: %s\n", address,
		))

		report := lookupServerVersion(path, "")

		require.NotNil(t, report)
		assert.NotEmpty(t, report.Error)
		assert.Empty(t, report.Version)
		assert.Equal(t, "http://"+address, report.APIEndpoint)
	})
}
