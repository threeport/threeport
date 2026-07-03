package cmd

import (
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"

	strcase "github.com/iancoleman/strcase"
	cobra "github.com/spf13/cobra"

	cli "github.com/threeport/threeport/pkg/cli/v0"
	client_v0 "github.com/threeport/threeport/pkg/client/v0"
)

var (
	eventsFor    string
	eventsOutput string
	eventsSort   string
	eventsLimit  int
)

const (
	eventShortAlias = "ev"
)

// GetEventsCmd represents the command 'tptctl get events'
var GetEventsCmd = &cobra.Command{
	Aliases: []string{"event", eventShortAlias},
	Example: `  # get all events
  tptctl get events

  # filter to a specific object (broad: any namespace/version)
  tptctl get events --for helm-workload-instance/my-app

  # narrow to one version
  tptctl get events --for v0.helm-workload-instance/my-app

  # narrow to one api namespace + version
  tptctl get events --for threeport.io/v0.helm-workload-instance/my-app`,
	Long: `Get events from the system.

Use --for [<namespace>/][<version>.]<kind>/<name> to filter events to a specific object. <namespace> and <version> are optional; <kind> and <name> are required. The kind is the kebab-case form of the API type name; the name is the object's Name field. Both core and module types are supported.

Use --sort to control row order: newest (default) puts the most recent activity at the top; oldest is reverse.

Use --limit N to cap the number of rows shown (after sort). The default of 0 means no cap.

Full event notes (including captured script stdout/stderr) can be viewed with -o yaml.`,
	PreRun: CommandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, requestedControlPlane := GetClientContext(cmd)

		// build query string from --for
		queryString, err := buildEventsQueryString(eventsFor)
		if err != nil {
			cli.Error("failed to build events query", err)
			os.Exit(1)
		}

		// fetch events
		events, err := client_v0.GetEventsJoinAttachedObjectReferenceByQueryString(apiClient, apiEndpoint, queryString)
		if err != nil {
			cli.Error("failed to retrieve events", err)
			os.Exit(1)
		}

		if len(*events) == 0 {
			cli.Info(fmt.Sprintf(
				"no events found that are currently managed by %s threeport control plane",
				requestedControlPlane,
			))
			os.Exit(0)
		}

		// sort by event time per --sort
		newestFirst := true
		switch eventsSort {
		case "newest":
			newestFirst = true
		case "oldest":
			newestFirst = false
		default:
			cli.Error("", fmt.Errorf("unrecognized sort: %s (expected newest or oldest)", eventsSort))
			os.Exit(1)
		}
		sort.SliceStable(*events, func(i, j int) bool {
			ti, tj := (*events)[i].EventTime, (*events)[j].EventTime
			if ti == nil || tj == nil {
				return ti != nil
			}
			if newestFirst {
				return ti.After(*tj)
			}
			return ti.Before(*tj)
		})

		// cap to --limit after sort so the cap respects user's chosen direction
		if eventsLimit > 0 && len(*events) > eventsLimit {
			truncated := (*events)[:eventsLimit]
			events = &truncated
		}

		// write output
		switch eventsOutput {
		case "tabular":
			if err := outputEventsTable(events); err != nil {
				cli.Error("failed to produce output", err)
				os.Exit(1)
			}
		case "yaml":
			if err := cli.YamlObjectOutput(*events); err != nil {
				cli.Error("failed to produce YAML output", err)
				os.Exit(1)
			}
		case "json":
			if err := cli.JsonObjectOutput(*events); err != nil {
				cli.Error("failed to produce JSON output", err)
				os.Exit(1)
			}
		default:
			cli.Error("", fmt.Errorf("unrecognized output format: %s", eventsOutput))
			os.Exit(1)
		}
	},
	Short:        "Get events from the system",
	SilenceUsage: true,
	Use:          "events",
}

func init() {
	GetCmd.AddCommand(GetEventsCmd)

	GetEventsCmd.Flags().StringVar(
		&eventsFor,
		"for", "", "Filter events by object, in the form <kind>/<name>. Kind is the kebab-case form of the API type name (e.g. machine-runtime-instance, router-definition).",
	)
	GetEventsCmd.Flags().StringVarP(
		&eventsOutput,
		"output", "o", "tabular", "Output format for events. One of: [yaml, json]",
	)
	GetEventsCmd.Flags().StringVar(
		&eventsSort,
		"sort", "newest", "Sort order. One of: [newest, oldest]",
	)
	GetEventsCmd.Flags().IntVar(
		&eventsLimit,
		"limit", 0, "Maximum number of events to display after sort. 0 means no cap.",
	)
	GetEventsCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}

// buildEventsQueryString turns the --for flag into the events query
// string. Accepts three input shapes, narrowing the query as more
// parts are supplied:
//
//	<kebab-kind>/<name>                                 - broad, any namespace/version
//	<version>.<kebab-kind>/<name>                       - narrow to one version
//	<namespace>/<version>.<kebab-kind>/<name>           - exact fully qualified type match
//
// The kind segment carries the optional version inline as
// "<version>.<kind>", mirroring the fully qualified type form. Empty flag returns
// empty string so the events list isn't filtered.
func buildEventsQueryString(forFlag string) (string, error) {
	// no filter requested - return empty so the caller queries every event
	if forFlag == "" {
		return "", nil
	}

	// split slash-delimited segments. parse right-to-left so the
	// optional namespace lands in the right slot
	parts := strings.Split(forFlag, "/")
	if len(parts) < 2 || len(parts) > 3 {
		return "", fmt.Errorf(
			"invalid --for value %q: expected [<namespace>/][<version>.]<kind>/<name>",
			forFlag,
		)
	}

	q := url.Values{}

	// last segment is always the object name
	name := parts[len(parts)-1]
	if name == "" {
		return "", fmt.Errorf("invalid --for value %q: empty name", forFlag)
	}
	q.Set("objectname", name)

	// second-to-last is the kind, optionally prefixed by "<version>."
	// e.g. "kubernetes-workload-instance" or "v0.kubernetes-workload-instance"
	kindPart := parts[len(parts)-2]
	if kindPart == "" {
		return "", fmt.Errorf("invalid --for value %q: empty kind", forFlag)
	}
	kind := kindPart
	if dotIdx := strings.Index(kindPart, "."); dotIdx >= 0 {
		// split version off the front
		version := kindPart[:dotIdx]
		kind = kindPart[dotIdx+1:]
		if version == "" || kind == "" {
			return "", fmt.Errorf("invalid --for value %q: empty version or kind around dot", forFlag)
		}
		q.Set("objectversion", version)
	}

	// kebab-case kind -> CamelCase TypeName segment of fully qualified type
	// ("kubernetes-workload-instance" -> "KubernetesWorkloadInstance")
	q.Set("objecttypename", strcase.ToCamel(kind))

	// third-to-last (if present) is the api namespace
	if len(parts) == 3 {
		namespace := parts[0]
		if namespace == "" {
			return "", fmt.Errorf("invalid --for value %q: empty namespace", forFlag)
		}
		q.Set("objectnamespace", namespace)
	}

	return q.Encode(), nil
}
