package cmd

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	strcase "github.com/iancoleman/strcase"
	cobra "github.com/spf13/cobra"

	apilib "github.com/threeport/threeport/pkg/api/lib/v0"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	client_v0 "github.com/threeport/threeport/pkg/client/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

var (
	eventsFor        string
	eventsObjectKind string
	eventsObjectId   string
	eventsApiGroup   string
	eventsName       string
	eventsReason     string
	eventsOutput     string
	eventsSort       string
	eventsLimit      int
	eventsTopLevel   bool
	eventsWide       bool
	eventsReverse    bool
	eventsSince      time.Duration
	eventsType       string
)

const (
	eventShortAlias = "ev"
)

// topLevelObjectKinds is the set of core kinds --top-level keeps. Sub-object
// kinds such as KubernetesWorkloadResourceInstance stay off the list.
var topLevelObjectKinds = map[string]bool{
	"KubernetesRuntimeDefinition":  true,
	"KubernetesRuntimeInstance":    true,
	"KubernetesWorkloadDefinition": true,
	"KubernetesWorkloadInstance":   true,
	"HelmWorkloadDefinition":       true,
	"HelmWorkloadInstance":         true,
	"ControlPlaneDefinition":       true,
	"ControlPlaneInstance":         true,
	"GatewayDefinition":            true,
	"GatewayInstance":              true,
	"DomainNameDefinition":         true,
	"DomainNameInstance":           true,
	"MachineRuntimeDefinition":     true,
	"MachineRuntimeInstance":       true,
	"MachineWorkloadDefinition":    true,
	"MachineWorkloadInstance":      true,
	"ObservabilityStackDefinition": true,
	"ObservabilityStackInstance":   true,
	"SecretDefinition":             true,
	"SecretInstance":               true,
	"TerraformDefinition":          true,
	"TerraformInstance":            true,
}

// GetEventsCmd represents the command 'tptctl get events'
var GetEventsCmd = &cobra.Command{
	Aliases: []string{"event", eventShortAlias},
	Example: `  # get all events
  tptctl get events

  # filter to a specific object (broad: any namespace/version)
  tptctl get events --for helm-workload-instance/my-app

  # narrow to one api namespace + version
  tptctl get events --for threeport.io/v0.helm-workload-instance/my-app

  # narrow to one version
  tptctl get events --for v0.helm-workload-instance/my-app

  # filter by kind alone across every object of that kind
  tptctl get events --object-kind helm-workload-instance

  # filter by API group / namespace alone
  tptctl get events --api-group threeport.io

  # filter by object name alone
  tptctl get events --name my-app

  # filter by object ID alone, across every kind carrying that ID
  tptctl get events --id 7

  # narrow an ID to one kind
  tptctl get events --object-kind kubernetes-workload-instance --id 7

  # filter by object name prefix (trailing * wildcard), covering a fleet
  # and every derived child whose name extends the fleet name
  tptctl get events --name 'myfleet2*'

  # combine narrow filters (kind + name, group + kind, etc.)
  tptctl get events --object-kind helm-workload-instance --name my-app

  # filter by Reason (case-sensitive CamelCase)
  tptctl get events --reason SuccessfulCreate

  # filter by Reason prefix (trailing * wildcard)
  tptctl get events --reason 'Create*'

  # show only events on top-level object kinds
  tptctl get events --top-level

  # filter to a subject by --for shape
  tptctl get events --for router-machine-set/demo1-router-set

  # name prefix inside a --for shape
  tptctl get events --for 'router-instance/myfleet2*'

  # only events within the last 5 minutes
  tptctl get events --since=5m

  # only Warning-type events
  tptctl get events --type=Warning

  # widen the MESSAGE column to the terminal width
  tptctl get events --wide

  # oldest events first, top-down causal read (equivalent to --sort=oldest)
  tptctl get events -r`,
	Long: `Get events from the system.

Use --for [<namespace>/][<version>.]<kind>/<name> to filter events to a specific object. <namespace> and <version> are optional; <kind> and <name> are required. The kind is the kebab-case form of the API type name; the name is the object's Name field. The name takes the same trailing-star prefix --name does. Both core and module types are supported.

Use --object-kind <kebab-kind> to filter events to a specific kind across every object of that kind. Mutually exclusive with --for; combinable with --api-group and --name.

Use --api-group <namespace> to filter events by API group / namespace alone (e.g. threeport.io). Mutually exclusive with --for; combinable with --object-kind and --name.

Use --name <name> to filter events by object name alone. Supports exact match (--name=my-app) or prefix match with a trailing star (--name='myfleet2*'). A prefix matches every object whose name starts with the token, across every object kind unless --object-kind or --api-group narrows it, so a fleet and the derived children named after it answer one query. Mutually exclusive with --for; combinable with --object-kind and --api-group.

Use --reason <reason> to filter events by Reason. Supports exact match (--reason=SuccessfulCreate) or prefix match with a trailing star (--reason='Create*'). Case-sensitive CamelCase. Applied server-side.

Use --top-level to drop events on sub-object kinds (e.g. GcpGceMachineRuntimeInstance, KubernetesWorkloadResourceInstance) and keep only events on top-level user-facing kinds.

Use --sort to control row order: newest (default) puts the most recent activity at the top matching kubectl's convention; oldest is reverse and lets a causal sequence read down. -r / --reverse is equivalent to --sort=oldest.

Use --limit N to cap the number of rows shown (after sort). The default of 0 means no cap.

Use --since=<duration> to filter events by recency (e.g. --since=10m). Zero disables the filter.

Use --type Normal|Warning to filter events by type. Empty disables the filter.

Use --wide to print the full note with no truncation, even when the terminal wraps.

AGE column: a single value is the event's age; a "first..last" span (e.g. 1h5m..1h4m) means the event was first observed at "first" ago and last observed at "last" ago.

Full event notes (including captured script stdout/stderr) can be viewed with -o yaml.`,
	PreRun: CommandPreRunFunc,
	Run: func(cmd *cobra.Command, args []string) {
		apiClient, _, apiEndpoint, requestedControlPlane := GetClientContext(cmd)

		// reject --for paired with any other subject selector
		if eventsFor != "" {
			switch {
			case eventsObjectKind != "":
				cli.Error("", errors.New("--for and --object-kind are mutually exclusive"))
				os.Exit(1)
			case eventsApiGroup != "":
				cli.Error("", errors.New("--for and --api-group are mutually exclusive"))
				os.Exit(1)
			case eventsName != "":
				cli.Error("", errors.New("--for and --name are mutually exclusive"))
				os.Exit(1)
			case eventsObjectId != "":
				cli.Error("", errors.New("--for and --id are mutually exclusive"))
				os.Exit(1)
			}
		}

		// reject --id paired with --name
		if eventsObjectId != "" && eventsName != "" {
			cli.Error("", errors.New("--id and --name are mutually exclusive"))
			os.Exit(1)
		}

		switch eventsType {
		case "", "Normal", "Warning":
		default:
			cli.Error("", fmt.Errorf("unrecognized type: %s (expected Normal or Warning)", eventsType))
			os.Exit(1)
		}

		// build the listing query from the subject flags
		queryString, err := buildEventsQueryString(
			eventsFor, eventsObjectKind, eventsApiGroup, eventsName, eventsObjectId, eventsReason,
		)
		if err != nil {
			cli.Error("failed to build events query", err)
			os.Exit(1)
		}
		if eventsType != "" {
			if queryString != "" {
				queryString += "&"
			}
			queryString += "type=" + url.QueryEscape(eventsType)
		}

		// fetch every matching page; --limit is a display cap after sort
		events, err := client_v0.GetEventsJoinAttachedObjectReferenceByQueryString(apiClient, apiEndpoint, queryString, 0)
		if err != nil {
			cli.Error("failed to retrieve events", err)
			os.Exit(1)
		}

		// drop events whose kind is not a top-level object
		if eventsTopLevel {
			filtered := make([]v0.Event, 0, len(*events))
			for _, e := range *events {
				if isTopLevelEvent(&e) {
					filtered = append(filtered, e)
				}
			}
			events = &filtered
		}

		// drop events older than --since
		if eventsSince > 0 {
			cutoff := time.Now().Add(-eventsSince)
			filtered := make([]v0.Event, 0, len(*events))
			for _, e := range *events {
				activity := eventActivityTime(&e)
				if activity != nil && activity.After(cutoff) {
					filtered = append(filtered, e)
				}
			}
			events = &filtered
		}

		if len(*events) == 0 {
			cli.Info(fmt.Sprintf(
				"no events found that are currently managed by %s threeport control plane",
				requestedControlPlane,
			))
			os.Exit(0)
		}

		// treat --reverse as --sort=oldest; reject it with an explicit --sort=newest
		if eventsReverse {
			if cmd.Flags().Changed("sort") && eventsSort == "newest" {
				cli.Error("", errors.New("--reverse and --sort=newest are mutually exclusive"))
				os.Exit(1)
			}
			eventsSort = "oldest"
		}

		// sort by event time per --sort
		newestFirst := false
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
			ti := eventActivityTime(&(*events)[i])
			tj := eventActivityTime(&(*events)[j])
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

		// print as tabular, yaml, or json
		switch eventsOutput {
		case "tabular":
			if err := outputEventsTable(events, eventsWide); err != nil {
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
		"for", "", "Filter events by object, in the form [<namespace>/][<version>.]<kind>/<name>. Kind is the kebab-case form of the API type name (e.g. machine-runtime-instance, router-definition). The name accepts a trailing * for a prefix match. Mutually exclusive with --object-kind.",
	)
	GetEventsCmd.Flags().StringVar(
		&eventsObjectKind,
		"object-kind", "", "Filter events by object kind alone (kebab-case form of the API type name, e.g. helm-workload-instance). Mutually exclusive with --for; combinable with --api-group and --name.",
	)
	GetEventsCmd.Flags().StringVar(
		&eventsObjectKind,
		"kind", "", "Alias for --object-kind.",
	)
	GetEventsCmd.Flags().StringVar(
		&eventsApiGroup,
		"api-group", "", "Filter events by API group / namespace (e.g. threeport.io). Mutually exclusive with --for; combinable with --object-kind and --name.",
	)
	GetEventsCmd.Flags().StringVar(
		&eventsName,
		"name", "", "Filter events by object name alone. Supports exact match (--name=my-app) or prefix match with trailing * (--name='myfleet2*'). Mutually exclusive with --for; combinable with --object-kind and --api-group.",
	)
	GetEventsCmd.Flags().StringVar(
		&eventsObjectId,
		"id", "", "Filter events by object ID, the numeric ID the object carries in the API. Read one off the ObjectID field of tptctl get events -o json, which is where tptctl surfaces it. Mutually exclusive with --for and --name; combinable with --object-kind and --api-group.",
	)
	GetEventsCmd.Flags().StringVar(
		&eventsReason,
		"reason", "", "Filter events by reason. Supports exact match (--reason=SuccessfulCreate) or prefix match with trailing * (--reason='Create*').",
	)
	GetEventsCmd.Flags().BoolVar(
		&eventsTopLevel,
		"top-level", false, "Show only events for top-level objects. Drops events for owned children (RouterMachineInstance under a Set, MachineRuntimeInstance under a RouterMachine, etc).",
	)
	GetEventsCmd.Flags().StringVarP(
		&eventsOutput,
		"output", "o", "tabular", "Output format for events. One of: [tabular, yaml, json]",
	)
	GetEventsCmd.Flags().StringVar(
		&eventsSort,
		"sort", "newest", "Sort order. One of: [oldest, newest]. Default newest places the most recent event at the top, matching kubectl's convention.",
	)
	GetEventsCmd.Flags().IntVar(
		&eventsLimit,
		"limit", 0, "Maximum number of events to display after sort. 0 means no cap.",
	)
	GetEventsCmd.Flags().DurationVar(
		&eventsSince,
		"since", 0, "Only show events with EventTime newer than the given duration ago (e.g. --since=10m, --since=1h). Zero means no time filter.",
	)
	GetEventsCmd.Flags().StringVar(
		&eventsType,
		"type", "", "Filter events by Type. One of: [Normal, Warning]. Empty means no filter.",
	)
	GetEventsCmd.Flags().BoolVar(
		&eventsWide,
		"wide", false, "Widen MESSAGE column to the terminal width.",
	)
	GetEventsCmd.Flags().BoolVarP(
		&eventsReverse,
		"reverse", "r", false, "Reverse sort order. Equivalent to --sort=oldest.",
	)
	GetEventsCmd.Flags().StringVarP(
		&cliArgs.ControlPlaneName,
		"control-plane-name", "i", "", "Optional. Name of control plane. Will default to current control plane if not provided.",
	)
}

// buildEventsQueryString encodes --for or the independent subject flags as listing
// query parameters. An empty result queries every event.
func buildEventsQueryString(forFlag, objectKindFlag, apiGroupFlag, nameFlag, objectIdFlag, reasonFlag string) (string, error) {
	// no filter requested - return empty so the caller queries every event
	if forFlag == "" && objectKindFlag == "" && apiGroupFlag == "" &&
		nameFlag == "" && objectIdFlag == "" && reasonFlag == "" {
		return "", nil
	}

	q := url.Values{}

	// encode --reason as exact match or trailing-star prefix
	if reasonFlag != "" {
		if err := setReasonQueryParam(q, reasonFlag); err != nil {
			return "", err
		}
	}

	// encode the independent subject flags, each AND-narrowing the match
	if forFlag == "" {
		if objectKindFlag != "" {
			q.Set("objecttypename", strcase.ToCamel(objectKindFlag))
		}
		if apiGroupFlag != "" {
			q.Set("objectnamespace", apiGroupFlag)
		}
		if nameFlag != "" {
			if err := setObjectNameQueryParam(q, "--name", nameFlag, nameFlag); err != nil {
				return "", err
			}
		}
		if objectIdFlag != "" {
			if _, err := strconv.ParseUint(objectIdFlag, 10, 64); err != nil {
				return "", fmt.Errorf("invalid --id value %q: expected a positive whole number", objectIdFlag)
			}
			q.Set("objectid", objectIdFlag)
		}
		return q.Encode(), nil
	}

	// parse --for right to left as [<namespace>/][<version>.]<kind>/<name>
	parts := strings.Split(forFlag, "/")
	if len(parts) < 2 || len(parts) > 3 {
		return "", fmt.Errorf(
			"invalid --for value %q: expected [<namespace>/][<version>.]<kind>/<name>",
			forFlag,
		)
	}

	// last segment is always the object name
	name := parts[len(parts)-1]
	if name == "" {
		return "", fmt.Errorf("invalid --for value %q: empty name", forFlag)
	}
	if err := setObjectNameQueryParam(q, "--for", forFlag, name); err != nil {
		return "", err
	}

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

// setObjectNameQueryParam writes objectname or objectnameprefix from a trailing
// star. flag and flagValue name the source in errors.
func setObjectNameQueryParam(q url.Values, flag, flagValue, name string) error {
	if strings.HasSuffix(name, "*") {
		prefix := strings.TrimSuffix(name, "*")
		if prefix == "" {
			return fmt.Errorf("invalid %s value %q: prefix is empty", flag, flagValue)
		}
		if strings.Contains(prefix, "*") {
			return fmt.Errorf("invalid %s value %q: star wildcard is only allowed as trailing character", flag, flagValue)
		}
		q.Set("objectnameprefix", prefix)
		return nil
	}
	if strings.Contains(name, "*") {
		return fmt.Errorf("invalid %s value %q: star wildcard is only allowed as trailing character", flag, flagValue)
	}
	q.Set("objectname", name)
	return nil
}

// setReasonQueryParam writes reason or reasonprefix from a trailing star, and
// rejects a star anywhere else.
func setReasonQueryParam(q url.Values, reasonFlag string) error {
	if strings.HasSuffix(reasonFlag, "*") {
		prefix := strings.TrimSuffix(reasonFlag, "*")
		if prefix == "" {
			return fmt.Errorf("invalid --reason value %q: prefix is empty", reasonFlag)
		}
		if strings.Contains(prefix, "*") {
			return fmt.Errorf("invalid --reason value %q: star wildcard is only allowed as trailing character", reasonFlag)
		}
		q.Set("reasonprefix", prefix)
		return nil
	}
	if strings.Contains(reasonFlag, "*") {
		return fmt.Errorf("invalid --reason value %q: star wildcard is only allowed as trailing character", reasonFlag)
	}
	q.Set("reason", reasonFlag)
	return nil
}

// isTopLevelEvent reports whether the event's subject kind is a top-level object.
// A missing or malformed type is not top-level.
func isTopLevelEvent(e *v0.Event) bool {
	rawType := util.DerefString(e.ObjectType)
	if rawType == "" {
		return false
	}
	_, _, typeName, ok := apilib.ParseQualifiedType(rawType)
	if !ok {
		return false
	}
	return topLevelObjectKinds[typeName]
}

// eventActivityTime returns the last observation when present, otherwise the first.
func eventActivityTime(e *v0.Event) *time.Time {
	if e.LastObservedTime != nil {
		return e.LastObservedTime
	}
	return e.EventTime
}
