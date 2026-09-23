package v0

import (
	"context"
	"fmt"
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"

	v0 "github.com/threeport/threeport/pkg/api/v0"
)

// An API object group such as kubernetes_workload maps to the controller
// kubernetes-workload-controller.

// ParseApis splits a comma-separated list of API object group names and drops
// blank entries. An empty string returns nil.
func ParseApis(value string) []string {
	// return nil for an empty value
	if value == "" {
		return nil
	}

	// split on commas
	parts := strings.Split(value, ",")

	// keep trimmed non-empty entries
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// SelectControllersByGroup returns the controllers for the named groups, in
// that order. No names returns allControllers unchanged. The single name none returns none.
func SelectControllersByGroup(
	groupNames []string,
	allControllers []*v0.ControlPlaneComponent,
) ([]*v0.ControlPlaneComponent, error) {
	// return every controller when no groups are named
	if len(groupNames) == 0 {
		return allControllers, nil
	}

	// return no controllers when the only group is none
	if len(groupNames) == 1 && groupNames[0] == "none" {
		return []*v0.ControlPlaneComponent{}, nil
	}

	// index controllers by name
	byName := make(map[string]*v0.ControlPlaneComponent, len(allControllers))
	for _, controller := range allControllers {
		byName[controller.Name] = controller
	}

	// select controllers in group-name order
	selected := make([]*v0.ControlPlaneComponent, 0, len(groupNames))
	for _, groupName := range groupNames {
		controllerName := controllerNameForGroup(groupName)
		controller, ok := byName[controllerName]
		if !ok {
			return nil, fmt.Errorf(
				"unknown api object group %q: valid choices are %s",
				groupName,
				strings.Join(ApiObjectGroupNames, ", "),
			)
		}
		selected = append(selected, controller)
	}

	return selected, nil
}

// DetectInstalledControllerNames returns controller names from installer-managed
// deployments. labeledCount includes the API server and agent that names omits.
func DetectInstalledControllerNames(
	kubeClient dynamic.Interface,
	namespace string,
) (names []string, labeledCount int, err error) {
	// list installer-managed deployments
	selector := fmt.Sprintf("%s=%s", LabelManagedBy, LabelManagedByValue)

	list, err := kubeClient.Resource(deploymentGVR).Namespace(namespace).List(
		context.Background(),
		metav1.ListOptions{LabelSelector: selector},
	)
	if err != nil {
		return nil, 0, fmt.Errorf(
			"failed to list installer-managed deployments in namespace %q: %w",
			namespace, err,
		)
	}

	names = make([]string, 0, len(list.Items))
	for _, item := range list.Items {
		deployName := item.GetName()
		// skip the API server and the agent
		if deployName == ThreeportAPIServiceResourceName || deployName == ThreeportAgentDeployName {
			continue
		}
		// drop the threeport- prefix so the name matches a controller
		stripped := strings.TrimPrefix(deployName, "threeport-")
		names = append(names, stripped)
	}

	// sort the names and return the unfiltered list length
	sort.Strings(names)
	return names, len(list.Items), nil
}

// SelectControllersForReinstall returns the controllers to reinstall and
// whether that set was read from the cluster. A supplied group list wins.
func SelectControllersForReinstall(
	kubeClient dynamic.Interface,
	namespace string,
	explicitGroups []string,
	allControllers []*v0.ControlPlaneComponent,
) ([]*v0.ControlPlaneComponent, []string, bool, error) {
	// use the supplied groups when any are set
	if len(explicitGroups) > 0 {
		selected, err := SelectControllersByGroup(explicitGroups, allControllers)
		if err != nil {
			return nil, nil, false, err
		}
		return selected, controllerNames(selected), false, nil
	}

	// read installed controller names
	detectedNames, labeledCount, err := DetectInstalledControllerNames(kubeClient, namespace)
	if err != nil {
		return nil, nil, true, fmt.Errorf("failed to detect installed controllers: %w", err)
	}

	// keep the full controller set when nothing is labeled
	if labeledCount == 0 {
		return allControllers, controllerNames(allControllers), true, nil
	}

	// index detected names
	wanted := make(map[string]struct{}, len(detectedNames))
	for _, name := range detectedNames {
		wanted[name] = struct{}{}
	}

	// keep installed controllers in allControllers order
	selected := make([]*v0.ControlPlaneComponent, 0, len(detectedNames))
	selectedNames := make([]string, 0, len(detectedNames))
	for _, controller := range allControllers {
		if _, ok := wanted[controller.Name]; ok {
			selected = append(selected, controller)
			selectedNames = append(selectedNames, controller.Name)
		}
	}

	return selected, selectedNames, true, nil
}

// controllerNames returns each controller's name in the same order.
func controllerNames(controllers []*v0.ControlPlaneComponent) []string {
	// collect controller names
	names := make([]string, 0, len(controllers))
	for _, controller := range controllers {
		names = append(names, controller.Name)
	}
	return names
}

// controllerNameForGroup maps an API object group name onto a controller name.
func controllerNameForGroup(groupName string) string {
	return strings.ReplaceAll(fmt.Sprintf("%s-controller", groupName), "_", "-")
}
