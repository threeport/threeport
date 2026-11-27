/*
Copyright © 2023 Threeport admin@threeport.io
*/
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	yaml "sigs.k8s.io/yaml"

	"github.com/threeport/threeport/internal/version"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	client_lib "github.com/threeport/threeport/pkg/client/lib/v0"
	client "github.com/threeport/threeport/pkg/client/v0"
)

// versionMismatchHelp points to how to get a tptctl that matches a control
// plane.  The Head builds page is a development doc, so it is named rather than
// linked.
const versionMismatchHelp = "See https://threeport.io/install/install-tptctl/ and docs/dev/head-builds.md."

var (
	versionClientOnly   bool
	versionOutputFormat string
)

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the tptctl and control plane versions",
	Long: `Print the version of tptctl, the client, and the version of the control plane
it is configured to use, the server.

tptctl uses its own version as the default image tag when it installs or
upgrades a control plane, so a tptctl that does not match the control plane it
manages quietly moves that control plane to different images.  When the two
versions differ, a warning is written to stderr.  The versions are compared as
strings and are not ordered.

The control plane is the current one in the Threeport config, or the one named
with --control-plane-name.  With no Threeport config, or no current control
plane, the server version is reported as none and the command succeeds.  If a
control plane is configured but its version cannot be read, the command prints
the client version, writes the error to stderr and exits 1.  --client prints only
the client version and does not read the config or contact a control plane.`,
	Run: func(cmd *cobra.Command, args []string) {
		exitCode := runVersion(
			cmd.OutOrStdout(),
			cmd.ErrOrStderr(),
			versionOptions{
				clientOnly: versionClientOnly,
				format:     versionOutputFormat,
			},
			version.GetVersion(),
			func() *serverVersionReport {
				return lookupServerVersion(cliArgs.CfgFile, cliArgs.ControlPlaneName)
			},
		)
		if exitCode != 0 {
			os.Exit(exitCode)
		}
	},
	SilenceUsage: true,
}

func init() {
	rootCmd.AddCommand(versionCmd)

	versionCmd.Flags().BoolVar(
		&versionClientOnly,
		"client", false, "Print only the client version.  Does not read the Threeport config or contact a control plane.",
	)
	versionCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
	versionCmd.Flags().StringVarP(
		&versionOutputFormat,
		"output", "o", "text", "Output format for the versions. One of: [text, yaml, json]",
	)
}

// versionOptions are the flags of the version command.
type versionOptions struct {
	// clientOnly is whether to print only the client version.
	clientOnly bool

	// format is the output format: text, json or yaml.
	format string
}

// versionReport is what the version command reports.  It is the structure of
// the json and yaml output, so the tags are the output's field names.
type versionReport struct {
	// ClientVersion is the version tptctl was built with.
	ClientVersion string `json:"clientVersion"`

	// Server is the control plane that was asked for its version.  It is nil
	// when no control plane is configured and when only the client was asked for.
	Server *serverVersionReport `json:"server"`

	// Match is whether the client and server versions are the same.  It is false
	// when the server version is not known.
	Match bool `json:"match"`
}

// serverVersionReport is the outcome of asking a control plane for its version.
type serverVersionReport struct {
	// ControlPlane is the name of the control plane.
	ControlPlane string `json:"controlPlane"`

	// APIEndpoint is the URL of the control plane's REST API.
	APIEndpoint string `json:"apiEndpoint"`

	// Version is the version the REST API reported.  It is empty when it could
	// not be read.
	Version string `json:"version"`

	// Error is why the version could not be read, and empty when it was.
	Error string `json:"error"`
}

// buildVersionReport compares the client version to the server's.  The two are
// compared exactly, apart from surrounding whitespace.  A server report that is
// nil or holds an error has no version to match.
func buildVersionReport(clientVersion string, server *serverVersionReport) versionReport {
	report := versionReport{ClientVersion: strings.TrimSpace(clientVersion)}
	if server == nil {
		return report
	}

	serverCopy := *server
	serverCopy.Version = strings.TrimSpace(serverCopy.Version)
	report.Server = &serverCopy
	report.Match = serverCopy.Error == "" && serverCopy.Version == report.ClientVersion

	return report
}

// versionMismatch is whether the server version was read and differs from the
// client's.
func (r versionReport) versionMismatch() bool {
	return r.Server != nil && r.Server.Error == "" && !r.Match
}

// mismatchWarning is the warning for a client that does not match the control
// plane, without the "Warning: " prefix.
func (r versionReport) mismatchWarning() string {
	return fmt.Sprintf(
		"tptctl version %s does not match control plane %s version %s.\n"+
			"Install a matching tptctl before installing, upgrading or changing this control plane. %s",
		r.ClientVersion,
		r.Server.ControlPlane,
		r.Server.Version,
		versionMismatchHelp,
	)
}

// readError describes a failure to read the server version.
func (s serverVersionReport) readError() string {
	if s.ControlPlane == "" {
		return fmt.Sprintf("failed to read the control plane version: %s", s.Error)
	}

	return fmt.Sprintf("failed to read the version of control plane %s: %s", s.ControlPlane, s.Error)
}

// renderVersionReport writes the report to w in the given format.  The text
// format has a server line only when there is a server version, or no control
// plane, to report; a failure to read the version is reported on stderr.
func renderVersionReport(w io.Writer, format string, report versionReport, clientOnly bool) error {
	switch format {
	case "json":
		// encoding/json rather than cli.JsonObjectOutput, which omits empty
		// fields: a null server, an empty error and a false match are part of
		// this output's structure
		output, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal version as JSON: %w", err)
		}
		_, err = fmt.Fprintln(w, string(output))
		return err
	case "yaml":
		output, err := yaml.Marshal(report)
		if err != nil {
			return fmt.Errorf("failed to marshal version as YAML: %w", err)
		}
		_, err = fmt.Fprint(w, string(output))
		return err
	case "text":
		if _, err := fmt.Fprintf(w, "Client Version: %s\n", report.ClientVersion); err != nil {
			return err
		}
		switch {
		case clientOnly:
		case report.Server == nil:
			_, err := fmt.Fprintln(w, "Server Version: none (no control plane in the Threeport config)")
			return err
		case report.Server.Error == "":
			_, err := fmt.Fprintf(
				w, "Server Version: %s (control plane %s)\n", report.Server.Version, report.Server.ControlPlane,
			)
			return err
		}
		return nil
	default:
		return fmt.Errorf("unrecognized output format: %s", format)
	}
}

// runVersion is the version command.  It writes the versions to out, and the
// warning for a mismatch and any error to errOut, so that out holds only the
// versions.  It returns the exit code: 1 when a control plane is configured but
// its version cannot be read, 0 otherwise, including on a mismatch.  lookup asks
// the control plane for its version and is not called with clientOnly.
func runVersion(
	out, errOut io.Writer,
	opts versionOptions,
	clientVersion string,
	lookup func() *serverVersionReport,
) int {
	// refuse a format before any request is made for it
	switch opts.format {
	case "text", "json", "yaml":
	default:
		fmt.Fprintf(errOut, "Error: unrecognized output format %q, must be one of: text, yaml, json\n", opts.format)
		return 1
	}

	var server *serverVersionReport
	if !opts.clientOnly {
		server = lookup()
	}
	report := buildVersionReport(clientVersion, server)

	if err := renderVersionReport(out, opts.format, report, opts.clientOnly); err != nil {
		fmt.Fprintf(errOut, "Error: failed to produce output: %s\n", err)
		return 1
	}

	if report.Server != nil && report.Server.Error != "" {
		fmt.Fprintf(errOut, "Error: %s\n", report.Server.readError())
		fmt.Fprintln(errOut, "Use --client to print only the client version without contacting a control plane.")
		return 1
	}

	if report.versionMismatch() {
		fmt.Fprintf(errOut, "Warning: %s\n", report.mismatchWarning())
	}

	return 0
}

// lookupServerVersion asks a control plane for its version.  The control plane
// is the one named, or the current one in the config when none is named.  The
// config is loaded here rather than in a pre-run function, which exits when
// there is no config or no current control plane, since neither is an error
// for this command.
//
// It returns nil when there is no control plane to ask, which is when there is
// no config file or no current control plane and none was named.  Any other
// failure is returned in the report's Error.
func lookupServerVersion(configPath, requestedControlPlane string) *serverVersionReport {
	loaded, err := cli.LoadConfigIfPresent(configPath)
	if err != nil {
		return &serverVersionReport{ControlPlane: requestedControlPlane, Error: err.Error()}
	}
	if !loaded {
		if requestedControlPlane != "" {
			return &serverVersionReport{
				ControlPlane: requestedControlPlane,
				Error:        "there is no Threeport config file to find the control plane in",
			}
		}
		return nil
	}

	threeportConfig, controlPlaneName, err := cli.GetThreeportConfig(requestedControlPlane)
	if err != nil {
		return &serverVersionReport{
			ControlPlane: requestedControlPlane,
			Error:        fmt.Sprintf("failed to get threeport config: %s", err),
		}
	}

	// tptctl down leaves the config without a current control plane
	if controlPlaneName == "" {
		return nil
	}

	report := &serverVersionReport{ControlPlane: controlPlaneName}

	if err := threeportConfig.ValidateControlPlaneName(controlPlaneName); err != nil {
		report.Error = err.Error()
		return report
	}
	apiEndpoint, err := threeportConfig.GetThreeportAPIEndpoint(controlPlaneName)
	if err != nil {
		report.Error = fmt.Sprintf("failed to get threeport API endpoint from config: %s", err)
		return report
	}
	apiClient, err := threeportConfig.GetHTTPClient(controlPlaneName)
	if err != nil {
		report.Error = fmt.Sprintf("failed to create threeport API client: %s", err)
		return report
	}

	report.APIEndpoint = client_lib.ApiUrl(apiClient, apiEndpoint)
	serverVersion, err := client.GetRestApiVersion(apiClient, apiEndpoint)
	if err != nil {
		report.Error = err.Error()
		return report
	}
	report.Version = serverVersion

	return report
}
